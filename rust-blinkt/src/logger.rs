//! Log lines in Go's standard `log` format: `YYYY/MM/DD HH:MM:SS message`
//! on stderr, so deploy checks that grep the Go output keep working.

use std::sync::OnceLock;

use time::macros::format_description;
use time::{OffsetDateTime, UtcOffset};

static OFFSET: OnceLock<UtcOffset> = OnceLock::new();

/// Captures the local UTC offset. Call before spawning threads: the offset
/// can only be read safely while the process is single-threaded. Without
/// zoneinfo (a `scratch` image) this is UTC, as with Go.
pub fn init() {
    OFFSET.get_or_init(|| UtcOffset::current_local_offset().unwrap_or(UtcOffset::UTC));
}

/// Formats one log line for the given time.
pub fn line(at: OffsetDateTime, msg: &str) -> String {
    let fmt = format_description!("[year]/[month]/[day] [hour]:[minute]:[second]");
    let ts = at.format(&fmt).unwrap_or_default();
    format!("{ts} {msg}")
}

/// Logs a message to stderr.
pub fn log(msg: &str) {
    let off = *OFFSET.get().unwrap_or(&UtcOffset::UTC);
    eprintln!("{}", line(OffsetDateTime::now_utc().to_offset(off), msg));
}

/// Logs a message and exits with status 1, like Go's log.Fatalf.
pub fn fatal(msg: &str) -> ! {
    log(msg);
    std::process::exit(1)
}

#[cfg(test)]
mod tests {
    use super::*;
    use time::macros::datetime;

    #[test]
    fn go_log_format() {
        assert_eq!(
            line(
                datetime!(2026-10-08 19:29:58.4 UTC),
                "blinkt: GPIO ready (x)"
            ),
            "2026/10/08 19:29:58 blinkt: GPIO ready (x)"
        );
    }
}
