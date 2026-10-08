//! Runtime settings derived from the environment and the config.

use std::time::Duration;

use crate::config::BlinktConfig;

/// Applies when the config omits frequency (the kubesim charts do): without
/// it fixed5 would redraw in a tight loop.
pub const DEFAULT_FREQUENCY: i64 = 1000;

/// How long fixed5 leaves its LEDs off between blinks, in milliseconds.
pub fn dark_delay(conf: &BlinktConfig) -> i64 {
    if conf.algorithm == "fixed5" {
        // We only leave the led dark for
        // a couple of milliseconds
        10
    } else {
        conf.frequency
    }
}

/// An entry outlives a few missed redraws before others stop drawing it.
pub fn entry_ttl(conf: &BlinktConfig) -> Duration {
    let ms = u64::try_from(conf.frequency + dark_delay(conf)).unwrap_or(0);
    3 * Duration::from_millis(ms) + Duration::from_secs(2)
}

/// The start-up line deploy checks grep for, e.g.
/// `blinkt: running algorithm=fixed5 frequency=1000ms config=... owner=... shared state=/etc/kubedge`.
pub fn running_line(
    conf: &BlinktConfig,
    config_path: &str,
    owner: &str,
    solo: Option<&str>,
    dir: &str,
) -> String {
    let mode = match solo {
        None => format!("shared state={dir}"),
        Some(why) => format!("solo ({why})"),
    };
    format!(
        "blinkt: running algorithm={} frequency={}ms config={config_path} owner={owner} {mode}",
        conf.algorithm, conf.frequency
    )
}

/// Names this process in the shared state: the pod name in Kubernetes.
pub fn owner() -> String {
    owner_from(
        std::env::var("BLINKT_OWNER").ok(),
        hostname(),
        std::process::id(),
    )
}

fn owner_from(env: Option<String>, host: Option<String>, pid: u32) -> String {
    env.filter(|o| !o.is_empty())
        .or(host)
        .unwrap_or_else(|| format!("pid-{pid}"))
}

fn hostname() -> Option<String> {
    let name = rustix::system::uname()
        .nodename()
        .to_string_lossy()
        .into_owned();
    (!name.is_empty()).then_some(name)
}

/// The host directory shared by every blinkt process on the node. Set but
/// empty means no shared state (solo).
pub fn state_dir() -> String {
    std::env::var("BLINKT_STATE_DIR").unwrap_or_else(|_| "/etc/kubedge".to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn conf(algorithm: &str, frequency: i64) -> BlinktConfig {
        BlinktConfig {
            algorithm: algorithm.into(),
            frequency,
            ..Default::default()
        }
    }

    #[test]
    fn ttl_for_fixed5_at_1000ms() {
        assert_eq!(
            entry_ttl(&conf("fixed5", 1000)),
            Duration::from_millis(5030)
        );
    }

    #[test]
    fn ttl_and_dark_time_for_other_algorithms() {
        assert_eq!(dark_delay(&conf("fixed", 500)), 500);
        assert_eq!(entry_ttl(&conf("fixed", 500)), Duration::from_millis(5000));
    }

    #[test]
    fn running_line_shared_and_solo() {
        let c = conf("fixed5", 1000);
        assert_eq!(
            running_line(&c, "/etc/kubedge/blinkt_conf.yaml", "kubesim-lte-5f7c9-abcde", None, "/etc/kubedge"),
            "blinkt: running algorithm=fixed5 frequency=1000ms config=/etc/kubedge/blinkt_conf.yaml owner=kubesim-lte-5f7c9-abcde shared state=/etc/kubedge"
        );
        assert_eq!(
            running_line(&c, "/c.yaml", "me", Some("no shared state directory configured"), ""),
            "blinkt: running algorithm=fixed5 frequency=1000ms config=/c.yaml owner=me solo (no shared state directory configured)"
        );
    }

    #[test]
    fn owner_precedence() {
        assert_eq!(owner_from(Some("me".into()), Some("pod-a".into()), 7), "me");
        assert_eq!(
            owner_from(Some(String::new()), Some("pod-a".into()), 7),
            "pod-a"
        );
        assert_eq!(owner_from(None, None, 7), "pid-7");
    }
}
