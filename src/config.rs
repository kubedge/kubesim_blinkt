//! The YAML config file the Helm charts mount at /etc/kubedge/blinkt_conf.yaml.

use std::collections::BTreeMap;
use std::fmt;

use serde::Deserialize;

use crate::ledstate::Pixel;

/// Where the Helm chart mounts the config; BLINKT_CONFIG overrides it.
pub const DEFAULT_PATH: &str = "/etc/kubedge/blinkt_conf.yaml";

/// Config keys; missing keys take zero values, unknown keys are ignored.
#[derive(Deserialize, Debug, Default, Clone, PartialEq, Eq)]
#[serde(default)]
pub struct BlinktConfig {
    pub algorithm: String,
    pub intensity: i64,
    pub frequency: i64,
    pub pixel0: Vec<i64>,
    pub pixel1: Vec<i64>,
    pub pixel2: Vec<i64>,
    pub pixel3: Vec<i64>,
    pub pixel4: Vec<i64>,
    pub pixel5: Vec<i64>,
    pub pixel6: Vec<i64>,
    pub pixel7: Vec<i64>,
}

#[derive(Debug)]
pub enum ConfigError {
    /// The file could not be read: logged as `blinkt: read config: ...`.
    Read(String),
    /// The YAML did not fit the keys: logged as `Unmarshal: ...`.
    Parse(String),
}

impl fmt::Display for ConfigError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ConfigError::Read(m) | ConfigError::Parse(m) => f.write_str(m),
        }
    }
}

/// The config file location.
pub fn path() -> String {
    match std::env::var("BLINKT_CONFIG") {
        Ok(p) if !p.is_empty() => p,
        _ => DEFAULT_PATH.to_string(),
    }
}

/// Reads and parses the config at `path`.
pub fn load(path: &str) -> Result<BlinktConfig, ConfigError> {
    let text = std::fs::read_to_string(path)
        .map_err(|e| ConfigError::Read(format!("open {path}: {e}")))?;
    parse(&text)
}

/// Parses config YAML; an empty document yields the zero config.
pub fn parse(text: &str) -> Result<BlinktConfig, ConfigError> {
    let value: serde_yaml_ng::Value =
        serde_yaml_ng::from_str(text).map_err(|e| ConfigError::Parse(e.to_string()))?;
    if value.is_null() {
        return Ok(BlinktConfig::default());
    }
    serde_yaml_ng::from_value(value).map_err(|e| ConfigError::Parse(e.to_string()))
}

impl BlinktConfig {
    /// The pixels this process owns: lists with at least three values, as
    /// red, green, blue at the configured intensity.
    pub fn pixels(&self) -> BTreeMap<usize, Pixel> {
        [
            &self.pixel0,
            &self.pixel1,
            &self.pixel2,
            &self.pixel3,
            &self.pixel4,
            &self.pixel5,
            &self.pixel6,
            &self.pixel7,
        ]
        .iter()
        .enumerate()
        .filter(|(_, rgb)| rgb.len() >= 3)
        .map(|(i, rgb)| {
            (
                i,
                Pixel {
                    r: rgb[0],
                    g: rgb[1],
                    b: rgb[2],
                    l: self.intensity,
                },
            )
        })
        .collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn sidecar_config() {
        let c = parse("---\nalgorithm: fixed5\nintensity: 5\npixel0: []\npixel6: [0, 0, 255]\n")
            .unwrap();
        assert_eq!(c.algorithm, "fixed5");
        assert_eq!(c.frequency, 0);
        let px = c.pixels();
        assert_eq!(px.len(), 1);
        assert_eq!(
            px[&6],
            Pixel {
                r: 0,
                g: 0,
                b: 255,
                l: 5
            }
        );
    }

    #[test]
    fn short_pixel_list_is_not_lit() {
        let c = parse("pixel2: [255]\npixel3: [1, 2, 3, 4]\n").unwrap();
        assert_eq!(c.pixels().keys().copied().collect::<Vec<_>>(), vec![3]);
    }

    #[test]
    fn unknown_keys_ignored_and_empty_document_is_zero() {
        assert_eq!(
            parse("release: red\nalgorithm: fixed\n").unwrap().algorithm,
            "fixed"
        );
        assert_eq!(parse("").unwrap(), BlinktConfig::default());
        assert_eq!(
            parse("---\n# comment only\n").unwrap(),
            BlinktConfig::default()
        );
    }

    #[test]
    fn invalid_yaml_is_a_parse_error() {
        assert!(matches!(
            parse("frequency: [oops"),
            Err(ConfigError::Parse(_))
        ));
        assert!(matches!(
            parse("frequency: fast"),
            Err(ConfigError::Parse(_))
        ));
    }

    #[test]
    fn missing_file_is_a_read_error_naming_the_path() {
        let err = load("/nonexistent/blinkt.yaml").unwrap_err();
        assert!(
            matches!(&err, ConfigError::Read(m) if m.starts_with("open /nonexistent/blinkt.yaml: "))
        );
    }

    #[test]
    fn path_honours_env() {
        // Single test touching the variable, so no cross-test races.
        std::env::remove_var("BLINKT_CONFIG");
        assert_eq!(path(), DEFAULT_PATH);
        std::env::set_var("BLINKT_CONFIG", "/tmp/x.yaml");
        assert_eq!(path(), "/tmp/x.yaml");
        std::env::set_var("BLINKT_CONFIG", "");
        assert_eq!(path(), DEFAULT_PATH);
        std::env::remove_var("BLINKT_CONFIG");
    }
}
