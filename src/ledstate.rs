//! Lets several blinkt processes on one node share the eight Blinkt! LEDs.
//! Each process (typically a sidecar in a kubesim pod) publishes only the
//! pixels it owns into a JSON file in a shared host directory. Under an
//! exclusive flock it merges every owner's pixels into one frame and draws it,
//! so co-located simulators each keep their own LED lit.
//!
//! Entries carry a TTL: a process that dies without withdrawing stops being
//! drawn once its entry expires and another process redraws.
//!
//! The file format and lock are shared with the Go 0.4.x implementation.

use std::collections::BTreeMap;
use std::fs::{self, File, OpenOptions};
use std::io::Write;
use std::os::unix::fs::OpenOptionsExt;
use std::path::{Path, PathBuf};
use std::time::Duration;

use rustix::fs::{flock, FlockOperation};
use serde::{Deserialize, Serialize};
use time::OffsetDateTime;

/// Number of LEDs on a Blinkt!.
pub const NUM_PIXELS: usize = 8;

const LOCK_NAME: &str = "blinkt.lock";
const STATE_NAME: &str = "blinkt_state.json";

pub type Error = Box<dyn std::error::Error + Send + Sync>;

/// One LED: colour 0-255 and luminance 0-31.
#[derive(Serialize, Deserialize, Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Pixel {
    #[serde(default)]
    pub r: i64,
    #[serde(default)]
    pub g: i64,
    #[serde(default)]
    pub b: i64,
    #[serde(default)]
    pub l: i64,
}

/// What gets drawn: one Pixel per LED, zero means dark.
pub type Frame = [Pixel; NUM_PIXELS];

/// One owner's contribution. Pixel keys are decimal LED indices.
#[derive(Serialize, Deserialize, Clone, Debug, PartialEq, Eq)]
pub struct Entry {
    #[serde(default)]
    pub pixels: Option<BTreeMap<String, Pixel>>,
    #[serde(with = "time::serde::rfc3339")]
    pub updated: OffsetDateTime,
    /// Nanoseconds, as Go's time.Duration.
    pub ttl: i64,
}

/// The content of the shared file.
#[derive(Serialize, Deserialize, Clone, Debug, Default, PartialEq, Eq)]
pub struct State {
    #[serde(default)]
    pub owners: BTreeMap<String, Entry>,
}

impl State {
    /// Drops entries that were not refreshed within their TTL.
    pub fn prune(&mut self, now: OffsetDateTime) {
        self.owners
            .retain(|_, e| (now - e.updated).whole_nanoseconds() <= i128::from(e.ttl));
    }

    /// Merges all owners. Owners apply in ascending byte order of their names,
    /// so when two claim the same LED the name sorting last wins.
    pub fn frame(&self) -> Frame {
        let mut f = Frame::default();
        for e in self.owners.values() {
            for (k, p) in e.pixels.iter().flatten() {
                if let Ok(i) = k.parse::<usize>() {
                    if i < NUM_PIXELS {
                        f[i] = *p;
                    }
                }
            }
        }
        f
    }
}

/// Builds an entry's pixel map from LED index to pixel.
pub fn pixels(px: &BTreeMap<usize, Pixel>) -> BTreeMap<String, Pixel> {
    px.iter().map(|(i, p)| (i.to_string(), *p)).collect()
}

type Render = Box<dyn FnMut(&Frame) -> Result<(), Error>>;
type Clock = Box<dyn Fn() -> OffsetDateTime>;

/// Publishes one owner's pixels and draws the merged frame.
pub struct Board {
    dir: PathBuf,
    owner: String,
    ttl: Duration,
    render: Render,
    now: Clock,
    lock: Option<File>,
    solo: State,
    solo_reason: Option<String>,
}

impl Board {
    /// Joins the shared state in `dir`. When `dir` is empty or `blinkt.lock`
    /// cannot be opened, the board runs solo: it draws only its own pixels and
    /// `solo_reason` says why.
    pub fn open(
        dir: &str,
        owner: &str,
        ttl: Duration,
        render: impl FnMut(&Frame) -> Result<(), Error> + 'static,
    ) -> Board {
        let mut b = Board {
            dir: PathBuf::from(dir),
            owner: owner.to_string(),
            ttl,
            render: Box::new(render),
            now: Box::new(OffsetDateTime::now_utc),
            lock: None,
            solo: State::default(),
            solo_reason: None,
        };
        if dir.is_empty() {
            b.solo_reason = Some("no shared state directory configured".into());
            return b;
        }
        let path = b.dir.join(LOCK_NAME);
        match OpenOptions::new()
            .create(true)
            .truncate(false)
            .read(true)
            .write(true)
            .mode(0o666)
            .open(&path)
        {
            Ok(f) => b.lock = Some(f),
            Err(e) => b.solo_reason = Some(format!("open {}: {e}", path.display())),
        }
        b
    }

    /// Replaces the clock; for tests.
    #[doc(hidden)]
    pub fn with_clock(mut self, now: impl Fn() -> OffsetDateTime + 'static) -> Board {
        self.now = Box::new(now);
        self
    }

    /// Why the board runs solo, or None when it shares state.
    pub fn solo_reason(&self) -> Option<&str> {
        self.solo_reason.as_deref()
    }

    /// Replaces this owner's pixels and redraws.
    pub fn publish(&mut self, px: &BTreeMap<usize, Pixel>) -> Result<(), Error> {
        let entry = Entry {
            pixels: Some(pixels(px)),
            updated: (self.now)(),
            ttl: i64::try_from(self.ttl.as_nanos()).unwrap_or(i64::MAX),
        };
        let owner = self.owner.clone();
        self.update(move |s| {
            s.owners.insert(owner, entry);
        })
    }

    /// Removes this owner's pixels and redraws whatever the others own.
    pub fn withdraw(&mut self) -> Result<(), Error> {
        let owner = self.owner.clone();
        self.update(move |s| {
            s.owners.remove(&owner);
        })
    }

    fn update(&mut self, change: impl FnOnce(&mut State)) -> Result<(), Error> {
        let Some(lock) = &self.lock else {
            change(&mut self.solo);
            return (self.render)(&self.solo.frame());
        };
        flock(lock, FlockOperation::LockExclusive).map_err(|e| format!("ledstate: lock: {e}"))?;
        let mut s = load(&self.dir);
        change(&mut s);
        s.prune((self.now)());
        let drawn = (self.render)(&s.frame());
        let saved = save(&self.dir, &s);
        let _ = flock(lock, FlockOperation::Unlock);
        match (drawn, saved) {
            (Err(d), Err(s)) => Err(format!("{d}\n{s}").into()),
            (Err(e), _) | (_, Err(e)) => Err(e),
            _ => Ok(()),
        }
    }
}

/// Reads the shared state; a missing or corrupt file starts empty.
pub fn load(dir: &Path) -> State {
    fs::read(dir.join(STATE_NAME))
        .ok()
        .and_then(|data| serde_json::from_slice(&data).ok())
        .unwrap_or_default()
}

fn save(dir: &Path, s: &State) -> Result<(), Error> {
    let data = serde_json::to_vec(s)?;
    let tmp = dir.join(format!("{STATE_NAME}.tmp"));
    let write = || -> std::io::Result<()> {
        let mut f = OpenOptions::new()
            .create(true)
            .write(true)
            .truncate(true)
            .mode(0o644)
            .open(&tmp)?;
        f.write_all(&data)?;
        fs::rename(&tmp, dir.join(STATE_NAME))
    };
    write().map_err(|e| format!("ledstate: save: {e}").into())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::cell::RefCell;
    use std::rc::Rc;
    use time::macros::datetime;

    const RED: Pixel = Pixel {
        r: 255,
        g: 0,
        b: 0,
        l: 5,
    };
    const GREEN: Pixel = Pixel {
        r: 0,
        g: 255,
        b: 0,
        l: 5,
    };
    const BLUE: Pixel = Pixel {
        r: 0,
        g: 0,
        b: 255,
        l: 5,
    };
    const DARK: Pixel = Pixel {
        r: 0,
        g: 0,
        b: 0,
        l: 0,
    };

    fn px(pairs: &[(usize, Pixel)]) -> BTreeMap<usize, Pixel> {
        pairs.iter().copied().collect()
    }

    /// Shared record of every frame drawn by any board.
    #[derive(Clone, Default)]
    struct Recorder(Rc<RefCell<Vec<Frame>>>);

    impl Recorder {
        fn render(&self) -> impl FnMut(&Frame) -> Result<(), Error> + 'static {
            let r = self.0.clone();
            move |f| {
                r.borrow_mut().push(*f);
                Ok(())
            }
        }
        fn last(&self) -> Frame {
            *self.0.borrow().last().expect("nothing drawn")
        }
    }

    fn board(dir: &Path, owner: &str, r: &Recorder) -> Board {
        let b = Board::open(
            dir.to_str().unwrap(),
            owner,
            Duration::from_secs(60),
            r.render(),
        );
        assert_eq!(b.solo_reason(), None);
        b
    }

    fn fixture() -> String {
        fs::read_to_string(format!(
            "{}/tests/fixtures/state_go.json",
            env!("CARGO_MANIFEST_DIR")
        ))
        .unwrap()
    }

    #[test]
    fn co_located_owners_keep_their_own_pixels() {
        let d = tempfile::tempdir().unwrap();
        let r = Recorder::default();
        let mut lte = board(d.path(), "kubesim-lte-abc", &r);
        let mut elte = board(d.path(), "kubesim-elte-def", &r);
        lte.publish(&px(&[(6, BLUE)])).unwrap();
        elte.publish(&px(&[(4, GREEN)])).unwrap();
        let f = r.last();
        assert_eq!((f[4], f[6]), (GREEN, BLUE));
        // elte blinking off must not turn lte's LED off.
        elte.publish(&px(&[])).unwrap();
        let f = r.last();
        assert_eq!((f[4], f[6]), (DARK, BLUE));
    }

    #[test]
    fn withdraw_leaves_others_lit() {
        let d = tempfile::tempdir().unwrap();
        let r = Recorder::default();
        let mut a = board(d.path(), "a", &r);
        let mut b = board(d.path(), "b", &r);
        a.publish(&px(&[(0, RED)])).unwrap();
        b.publish(&px(&[(7, BLUE)])).unwrap();
        b.withdraw().unwrap();
        let f = r.last();
        assert_eq!((f[0], f[7]), (RED, DARK));
    }

    #[test]
    fn expired_owner_is_not_drawn() {
        let d = tempfile::tempdir().unwrap();
        let r = Recorder::default();
        let t0 = datetime!(2026-10-08 12:00 UTC);
        let mut crashed = Board::open(
            d.path().to_str().unwrap(),
            "crashed",
            Duration::from_secs(5),
            r.render(),
        )
        .with_clock(move || t0);
        crashed.publish(&px(&[(3, RED)])).unwrap();
        drop(crashed); // dies without withdraw
        let mut alive =
            board(d.path(), "alive", &r).with_clock(move || t0 + Duration::from_secs(6));
        alive.publish(&px(&[(5, GREEN)])).unwrap();
        let f = r.last();
        assert_eq!((f[3], f[5]), (DARK, GREEN));
        assert!(!load(d.path()).owners.contains_key("crashed"));
    }

    #[test]
    fn conflicting_claims_resolve_by_owner_name() {
        let mut s = State::default();
        for (name, p) in [("kubesim-nr", RED), ("kubesim-5gc", BLUE)] {
            s.owners.insert(
                name.into(),
                Entry {
                    pixels: Some(pixels(&px(&[(7, p)]))),
                    updated: datetime!(2026-10-08 12:00 UTC),
                    ttl: 1,
                },
            );
        }
        assert_eq!(s.frame()[7], RED);
    }

    #[test]
    fn out_of_range_and_non_numeric_pixels_ignored() {
        let s: State = serde_json::from_str(
            r#"{"owners":{"x":{"pixels":{"-1":{"r":9},"8":{"r":9},"z":{"r":9},"2":{"g":255,"l":5}},"updated":"2026-10-08T12:00:00Z","ttl":1}}}"#,
        )
        .unwrap();
        let f = s.frame();
        assert_eq!(f[2], GREEN);
        assert!(f.iter().enumerate().all(|(i, p)| i == 2 || *p == DARK));
    }

    #[test]
    fn concurrent_publishers_never_lose_each_other() {
        let d = tempfile::tempdir().unwrap();
        let dir = d.path().to_str().unwrap().to_string();
        let handles: Vec<_> = (0..4)
            .map(|i| {
                let dir = dir.clone();
                std::thread::spawn(move || {
                    let mut b = Board::open(
                        &dir,
                        &format!("owner-{i}"),
                        Duration::from_secs(60),
                        |_: &Frame| Ok(()),
                    );
                    for _ in 0..25 {
                        b.publish(&px(&[(i, RED)])).unwrap();
                    }
                })
            })
            .collect();
        for h in handles {
            h.join().unwrap();
        }
        let r = Recorder::default();
        board(d.path(), "owner-0", &r)
            .publish(&px(&[(0, RED)]))
            .unwrap();
        let f = r.last();
        assert!((0..4).all(|i| f[i] == RED), "lost a pixel: {f:?}");
    }

    #[test]
    fn corrupt_state_file_starts_empty() {
        let d = tempfile::tempdir().unwrap();
        fs::write(d.path().join(STATE_NAME), "{not json").unwrap();
        let r = Recorder::default();
        board(d.path(), "a", &r)
            .publish(&px(&[(1, GREEN)]))
            .unwrap();
        assert_eq!(r.last()[1], GREEN);
    }

    #[test]
    fn null_pixels_and_owners_are_accepted() {
        let s: State = serde_json::from_str(
            r#"{"owners":{"x":{"pixels":null,"updated":"2026-10-08T12:00:00Z","ttl":1}}}"#,
        )
        .unwrap();
        assert_eq!(s.frame(), Frame::default());
        let d = tempfile::tempdir().unwrap();
        fs::write(d.path().join(STATE_NAME), r#"{"owners":null}"#).unwrap();
        assert!(load(d.path()).owners.is_empty());
    }

    #[test]
    fn solo_when_directory_unusable() {
        let d = tempfile::tempdir().unwrap();
        let r = Recorder::default();
        let missing = d.path().join("missing");
        let mut b = Board::open(
            missing.to_str().unwrap(),
            "a",
            Duration::from_secs(60),
            r.render(),
        );
        assert!(b.solo_reason().unwrap().contains("missing"));
        b.publish(&px(&[(2, BLUE)])).unwrap();
        assert_eq!(r.last()[2], BLUE);
        let empty = Board::open("", "a", Duration::from_secs(60), r.render());
        assert_eq!(
            empty.solo_reason(),
            Some("no shared state directory configured")
        );
    }

    #[test]
    fn render_error_is_returned_and_state_still_saved() {
        let d = tempfile::tempdir().unwrap();
        let mut b = Board::open(
            d.path().to_str().unwrap(),
            "a",
            Duration::from_secs(60),
            |_: &Frame| Err("gpio gone".into()),
        );
        let err = b.publish(&px(&[(0, RED)])).unwrap_err();
        assert_eq!(err.to_string(), "gpio gone");
        assert!(load(d.path()).owners.contains_key("a"));
    }

    #[test]
    fn reads_go_written_state() {
        let s: State = serde_json::from_str(&fixture()).unwrap();
        let lte = &s.owners["kubesim-lte-7d9f"];
        assert_eq!(lte.updated, datetime!(2026-10-08 19:00:00.123456789 UTC));
        assert_eq!(lte.ttl, 5_030_000_000);
        assert_eq!(
            s.owners["kubesim-elte-2b1c"].updated,
            datetime!(2026-10-08 19:00:00.5 UTC)
        );
        let f = s.frame();
        assert_eq!(f[6], BLUE);
        assert!(f.iter().enumerate().all(|(i, p)| i == 6 || *p == DARK));
    }

    #[test]
    fn writes_byte_identical_to_go() {
        let s: State = serde_json::from_str(&fixture()).unwrap();
        assert_eq!(serde_json::to_string(&s).unwrap(), fixture().trim());
    }

    #[test]
    fn spec_example_entry_format() {
        let mut s = State::default();
        s.owners.insert(
            "kubesim-lte-7d9f".into(),
            Entry {
                pixels: Some(pixels(&px(&[(6, BLUE)]))),
                updated: datetime!(2026-10-08 19:00:00.123456789 UTC),
                ttl: 5_000_000_000,
            },
        );
        assert_eq!(
            serde_json::to_string(&s).unwrap(),
            r#"{"owners":{"kubesim-lte-7d9f":{"pixels":{"6":{"r":0,"g":0,"b":255,"l":5}},"updated":"2026-10-08T19:00:00.123456789Z","ttl":5000000000}}}"#
        );
    }

    #[test]
    fn save_is_atomic_rename_with_0644() {
        use std::os::unix::fs::PermissionsExt;
        let d = tempfile::tempdir().unwrap();
        save(d.path(), &State::default()).unwrap();
        assert!(!d.path().join(format!("{STATE_NAME}.tmp")).exists());
        let mode = fs::metadata(d.path().join(STATE_NAME))
            .unwrap()
            .permissions()
            .mode()
            & 0o777;
        assert_eq!(mode & !0o022, 0o644 & !0o022);
    }
}
