//! blinkt5: lights this process's pixels on a shared Blinkt!.

use std::collections::BTreeMap;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::thread::sleep;
use std::time::Duration;

use kubesim_blinkt::config::{self, BlinktConfig, ConfigError};
use kubesim_blinkt::led_output::{self, BUSY_RETRY, BUSY_TIMEOUT};
use kubesim_blinkt::ledstate::{Board, Frame, Pixel};
use kubesim_blinkt::logger::{self, fatal, log};
use kubesim_blinkt::runtime::{
    dark_delay, entry_ttl, owner, running_line, state_dir, DEFAULT_FREQUENCY,
};
use signal_hook::consts::{SIGINT, SIGTERM};
use signal_hook::iterator::Signals;

fn delay(ms: i64) {
    sleep(Duration::from_millis(u64::try_from(ms).unwrap_or(0)));
}

/// Shows this process's pixels and aborts on failure, so a broken GPIO setup
/// shows up in the pod log instead of as dark LEDs.
fn publish(board: &mut Board, px: &BTreeMap<usize, Pixel>) {
    if let Err(e) = board.publish(px) {
        fatal(&format!("blinkt: {e}"));
    }
}

fn blinkt5(running: &AtomicBool, board: &mut Board) {
    let mut px = BTreeMap::new();
    while running.load(Ordering::SeqCst) {
        px.insert(
            fastrand::usize(0..8),
            Pixel {
                r: fastrand::i64(0..256),
                g: fastrand::i64(0..256),
                b: fastrand::i64(0..256),
                l: fastrand::i64(0..3),
            },
        );
        publish(board, &px);
        delay(60);
    }
}

fn fixed5(running: &AtomicBool, board: &mut Board, conf: &BlinktConfig) {
    let on = conf.pixels();
    let off = BTreeMap::new();
    while running.load(Ordering::SeqCst) {
        publish(board, &on);
        delay(conf.frequency);
        publish(board, &off);
        delay(dark_delay(conf));
    }
}

fn main() {
    logger::init();
    let running = Arc::new(AtomicBool::new(true));
    // initialise getout
    let mut signals =
        Signals::new([SIGINT, SIGTERM]).unwrap_or_else(|e| fatal(&format!("blinkt: signals: {e}")));
    let flag = running.clone();
    std::thread::spawn(move || {
        if let Some(sig) = signals.forever().next() {
            match sig {
                SIGINT => println!("Stopping on Interrupt"),
                _ => println!("Stopping on Terminate"),
            }
            flag.store(false, Ordering::SeqCst);
        }
    });

    // Check the GPIO lines once, then hand them back: other blinkt
    // processes on this node may be drawing.
    let source = led_output::system_lines();
    match led_output::acquire(source.as_ref(), BUSY_TIMEOUT, BUSY_RETRY) {
        Ok(lines) => log(&format!("blinkt: GPIO ready ({})", lines.describe())),
        Err(e) => fatal(&format!("blinkt: GPIO setup failed: {e}")),
    }

    let path = config::path();
    let mut conf = match config::load(&path) {
        Ok(c) => c,
        // Without a config fixed5 lights nothing, so fail visibly instead.
        Err(ConfigError::Read(e)) => fatal(&format!("blinkt: read config: {e}")),
        Err(ConfigError::Parse(e)) => fatal(&format!("Unmarshal: {e}")),
    };
    if conf.frequency <= 0 {
        conf.frequency = DEFAULT_FREQUENCY;
    }

    // Draw one merged frame, holding the GPIO lines only for that frame.
    let draw = move |f: &Frame| -> Result<(), kubesim_blinkt::ledstate::Error> {
        let mut lines = led_output::acquire(source.as_ref(), BUSY_TIMEOUT, BUSY_RETRY)?;
        lines.draw(f)?;
        Ok(())
    };
    let dir = state_dir();
    let me = owner();
    let mut board = Board::open(&dir, &me, entry_ttl(&conf), draw);
    // Deploy verification greps for this line.
    log(&running_line(&conf, &path, &me, board.solo_reason(), &dir));

    if conf.algorithm == "blinkt5" {
        blinkt5(&running, &mut board);
    } else {
        fixed5(&running, &mut board, &conf);
    }
    println!("Stopping");
    // Turn off only this process's LEDs; others on the node keep theirs.
    if let Err(e) = board.withdraw() {
        log(&format!("blinkt: exit: {e}"));
    }
}
