//! Drives the Blinkt! by bit-banging BCM GPIO23 (data) and GPIO24 (clock)
//! through the Linux GPIO character device. Unlike memory-mapped /dev/gpiomem
//! or sysfs GPIO numbers, this works unchanged on 32 and 64 bit kernels and on
//! kernels that number sysfs GPIOs from 512.

use std::fmt;
use std::thread::sleep;
use std::time::{Duration, Instant};

use crate::ledstate::Frame;

/// BCM GPIO lines wired to the Blinkt! header.
pub const DAT_OFFSET: u32 = 23;
pub const CLK_OFFSET: u32 = 24;
/// Chip used when the kernel does not publish gpio-line-names.
pub const DEFAULT_CHIP: &str = "gpiochip0";
pub const CONSUMER: &str = "kubesim_blinkt";

/// How long acquisition waits for lines held by another process.
pub const BUSY_TIMEOUT: Duration = Duration::from_secs(2);
pub const BUSY_RETRY: Duration = Duration::from_millis(5);

/// A GPIO failure; `Busy` means another process holds the line (EBUSY).
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum GpioError {
    Busy(String),
    Other(String),
}

impl fmt::Display for GpioError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            GpioError::Busy(m) | GpioError::Other(m) => f.write_str(m),
        }
    }
}

impl std::error::Error for GpioError {}

/// One output line; tests substitute a recording fake.
pub trait OutputPin {
    fn set(&mut self, high: bool) -> Result<(), GpioError>;
}

/// Opens one output line for a BCM GPIO and describes where it lives.
pub trait LineSource {
    fn request(&self, bcm: u32) -> Result<(Box<dyn OutputPin>, String), GpioError>;
}

/// The bit stream for one frame: 32 start bits, per LED `0xE0|lum`, blue,
/// green, red (MSB first), then 36 end bits. Values are masked to the
/// hardware range.
pub fn encode(frame: &Frame) -> Vec<bool> {
    let mut bits = Vec::with_capacity(32 + frame.len() * 32 + 36);
    bits.extend(std::iter::repeat_n(false, 32));
    for p in frame {
        for byte in [0xE0 | (p.l & 31), p.b & 255, p.g & 255, p.r & 255] {
            for i in (0..8).rev() {
                bits.push((byte >> i) & 1 == 1);
            }
        }
    }
    bits.extend(std::iter::repeat_n(false, 36));
    bits
}

/// Clocks a frame out: data is set before each rising clock edge.
pub fn write_frame(
    dat: &mut dyn OutputPin,
    clk: &mut dyn OutputPin,
    frame: &Frame,
) -> Result<(), GpioError> {
    for bit in encode(frame) {
        dat.set(bit).map_err(|e| prefix("data", e))?;
        clk.set(true).map_err(|e| prefix("clock high", e))?;
        clk.set(false).map_err(|e| prefix("clock low", e))?;
    }
    Ok(())
}

fn prefix(what: &str, e: GpioError) -> GpioError {
    GpioError::Other(format!("periBlink: {what}: {e}"))
}

/// Data and clock lines held for one or more frames; dropping releases them.
pub struct Lines {
    dat: Box<dyn OutputPin>,
    clk: Box<dyn OutputPin>,
    desc: String,
}

impl Lines {
    /// e.g. `data=gpiochip0:23 clock=gpiochip0:24`
    pub fn describe(&self) -> &str {
        &self.desc
    }

    pub fn draw(&mut self, frame: &Frame) -> Result<(), GpioError> {
        write_frame(self.dat.as_mut(), self.clk.as_mut(), frame)
    }
}

/// Requests both lines, retrying while another process holds them:
/// several blinkt sidecars on one node take turns at the lines frame by frame.
pub fn acquire(
    src: &dyn LineSource,
    timeout: Duration,
    retry: Duration,
) -> Result<Lines, GpioError> {
    let (dat, d_desc) = request_while_busy(src, DAT_OFFSET, timeout, retry)?;
    let (clk, c_desc) = request_while_busy(src, CLK_OFFSET, timeout, retry)?;
    Ok(Lines {
        dat,
        clk,
        desc: format!("data={d_desc} clock={c_desc}"),
    })
}

fn request_while_busy(
    src: &dyn LineSource,
    bcm: u32,
    timeout: Duration,
    retry: Duration,
) -> Result<(Box<dyn OutputPin>, String), GpioError> {
    let deadline = Instant::now() + timeout;
    loop {
        match src.request(bcm) {
            Err(GpioError::Busy(_)) if Instant::now() < deadline => sleep(retry),
            other => return other,
        }
    }
}

/// The production line source for this platform.
pub fn system_lines() -> Box<dyn LineSource> {
    Box::new(platform::CdevSource)
}

#[cfg(target_os = "linux")]
mod platform {
    use std::path::{Path, PathBuf};

    use gpiocdev::line::Value;
    use gpiocdev::Request;

    use super::{GpioError, LineSource, OutputPin, CONSUMER, DEFAULT_CHIP};

    pub struct CdevSource;

    struct CdevPin {
        req: Request,
        offset: u32,
    }

    impl OutputPin for CdevPin {
        fn set(&mut self, high: bool) -> Result<(), GpioError> {
            let v = if high { Value::Active } else { Value::Inactive };
            self.req
                .set_value(self.offset, v)
                .map_err(|e| GpioError::Other(e.to_string()))
        }
    }

    /// Resolves a BCM GPIO by its device-tree line name ("GPIO23"), falling
    /// back to the same offset on the default chip.
    fn find_line(bcm: u32) -> (PathBuf, u32) {
        match gpiocdev::find_named_line(&format!("GPIO{bcm}")) {
            Some(found) => (found.chip, found.info.offset),
            None => (Path::new("/dev").join(DEFAULT_CHIP), bcm),
        }
    }

    fn is_busy(e: &gpiocdev::Error) -> bool {
        let busy = rustix::io::Errno::BUSY.raw_os_error();
        matches!(e,
            gpiocdev::Error::Uapi(_, gpiocdev_uapi::Error::Os(gpiocdev_uapi::Errno(n)))
            | gpiocdev::Error::Os(gpiocdev_uapi::Errno(n)) if *n == busy)
    }

    impl LineSource for CdevSource {
        fn request(&self, bcm: u32) -> Result<(Box<dyn OutputPin>, String), GpioError> {
            let (chip, offset) = find_line(bcm);
            let name = chip
                .file_name()
                .map(|n| n.to_string_lossy().into_owned())
                .unwrap_or_else(|| chip.display().to_string());
            match Request::builder()
                .on_chip(&chip)
                .with_consumer(CONSUMER)
                .with_line(offset)
                .as_output(Value::Inactive)
                .request()
            {
                Ok(req) => Ok((
                    Box::new(CdevPin { req, offset }),
                    format!("{name}:{offset}"),
                )),
                Err(e) => {
                    let msg =
                        format!("periBlink: request {name} line {offset} (BCM GPIO{bcm}): {e}");
                    Err(if is_busy(&e) {
                        GpioError::Busy(msg)
                    } else {
                        GpioError::Other(msg)
                    })
                }
            }
        }
    }
}

#[cfg(not(target_os = "linux"))]
mod platform {
    use super::{GpioError, LineSource, OutputPin};

    pub struct CdevSource;

    impl LineSource for CdevSource {
        fn request(&self, _bcm: u32) -> Result<(Box<dyn OutputPin>, String), GpioError> {
            Err(GpioError::Other(
                "periBlink: GPIO character device is only available on Linux".into(),
            ))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ledstate::Pixel;
    use std::cell::RefCell;
    use std::rc::Rc;

    /// Records the data level latched on each rising clock edge.
    #[derive(Default)]
    struct Bus {
        data: bool,
        clock: bool,
        bits: Vec<bool>,
    }

    struct Data(Rc<RefCell<Bus>>);
    struct Clock(Rc<RefCell<Bus>>);

    impl OutputPin for Data {
        fn set(&mut self, high: bool) -> Result<(), GpioError> {
            self.0.borrow_mut().data = high;
            Ok(())
        }
    }

    impl OutputPin for Clock {
        fn set(&mut self, high: bool) -> Result<(), GpioError> {
            let mut b = self.0.borrow_mut();
            if high && !b.clock {
                let d = b.data;
                b.bits.push(d);
            }
            b.clock = high;
            Ok(())
        }
    }

    fn clocked(frame: &Frame) -> String {
        let bus = Rc::new(RefCell::new(Bus::default()));
        let (mut d, mut c) = (Data(bus.clone()), Clock(bus.clone()));
        write_frame(&mut d, &mut c, frame).unwrap();
        let bits: String = bus
            .borrow()
            .bits
            .iter()
            .map(|b| if *b { '1' } else { '0' })
            .collect();
        bits
    }

    fn fixture(name: &str) -> String {
        std::fs::read_to_string(format!(
            "{}/../tests/fixtures/{name}",
            env!("CARGO_MANIFEST_DIR")
        ))
        .unwrap()
        .trim()
        .to_string()
    }

    fn px(r: i64, g: i64, b: i64, l: i64) -> Pixel {
        Pixel { r, g, b, l }
    }

    #[test]
    fn one_pixel_matches_go_fixture() {
        let mut f = Frame::default();
        f[0] = px(0x12, 0x34, 0x56, 7);
        assert_eq!(clocked(&f), fixture("frame_one_pixel.bits"));
    }

    #[test]
    fn masked_values_match_go_fixture() {
        let mut f = Frame::default();
        f[2] = px(261, -1, 300, 40);
        assert_eq!(clocked(&f), fixture("frame_masked_pixel2.bits"));
    }

    #[test]
    fn dark_and_shared_frames_match_go_fixtures() {
        assert_eq!(clocked(&Frame::default()), fixture("frame_dark.bits"));
        let mut f = Frame::default();
        f[4] = px(0, 255, 0, 5);
        f[6] = px(0, 0, 255, 5);
        assert_eq!(clocked(&f), fixture("frame_lte_elte.bits"));
    }

    #[test]
    fn frame_layout() {
        let mut f = Frame::default();
        f[0] = px(0x12, 0x34, 0x56, 7);
        let bits = encode(&f);
        assert_eq!(bits.len(), 32 + 8 * 32 + 36);
        assert!(bits[..32].iter().all(|b| !b));
        assert!(bits[bits.len() - 36..].iter().all(|b| !b));
    }

    /// Line source whose first `busy` requests fail with EBUSY.
    struct FakeSource {
        busy: RefCell<u32>,
        err: Option<GpioError>,
        calls: RefCell<u32>,
    }

    struct Null;
    impl OutputPin for Null {
        fn set(&mut self, _: bool) -> Result<(), GpioError> {
            Ok(())
        }
    }

    impl LineSource for FakeSource {
        fn request(&self, bcm: u32) -> Result<(Box<dyn OutputPin>, String), GpioError> {
            *self.calls.borrow_mut() += 1;
            if let Some(e) = &self.err {
                return Err(e.clone());
            }
            let mut busy = self.busy.borrow_mut();
            if *busy > 0 {
                *busy -= 1;
                return Err(GpioError::Busy(format!("line {bcm} busy")));
            }
            Ok((Box::new(Null), format!("chip:{bcm}")))
        }
    }

    fn source(busy: u32, err: Option<GpioError>) -> FakeSource {
        FakeSource {
            busy: RefCell::new(busy),
            err,
            calls: RefCell::new(0),
        }
    }

    #[test]
    fn busy_lines_freed_within_window() {
        let src = source(3, None);
        let lines = acquire(&src, BUSY_TIMEOUT, Duration::from_millis(1)).unwrap();
        assert_eq!(lines.describe(), "data=chip:23 clock=chip:24");
    }

    #[test]
    fn busy_lines_held_too_long() {
        let src = source(u32::MAX, None);
        let err = acquire(&src, Duration::from_millis(20), Duration::from_millis(1))
            .err()
            .unwrap();
        assert!(matches!(err, GpioError::Busy(_)));
    }

    #[test]
    fn other_errors_fail_at_once() {
        let missing = GpioError::Other(
            "periBlink: request gpiochip0 line 23 (BCM GPIO23): no such file".into(),
        );
        let src = source(0, Some(missing.clone()));
        let err = acquire(&src, BUSY_TIMEOUT, BUSY_RETRY).err().unwrap();
        assert_eq!(err, missing);
        assert_eq!(*src.calls.borrow(), 1);
    }
}
