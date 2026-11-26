use std::fmt;

use serde::{Deserialize, Deserializer, Serialize, Serializer};

/// The pane SDK version this build speaks.
pub const SDK_VERSION: SdkVersion = SdkVersion { major: 1, minor: 0 };

/// A pane SDK version, `<major>.<minor>`, as declared in extension manifests (`sdk: ^1`).
///
/// A major bump breaks frames; a minor bump only adds frame kinds.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct SdkVersion {
    major: u32,
    minor: u32,
}

impl SdkVersion {
    /// Version `major.minor`.
    #[must_use]
    pub fn new(
        major: u32,
        minor: u32,
    ) -> Self {
        Self { major, minor }
    }

    pub(crate) fn major(self) -> u32 {
        self.major
    }
}

impl fmt::Display for SdkVersion {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        write!(f, "{}.{}", self.major, self.minor)
    }
}

impl Serialize for SdkVersion {
    fn serialize<S: Serializer>(
        &self,
        serializer: S,
    ) -> Result<S::Ok, S::Error> {
        serializer.collect_str(self)
    }
}

impl<'de> Deserialize<'de> for SdkVersion {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        let text = String::deserialize(deserializer)?;
        parse(&text).ok_or_else(|| serde::de::Error::custom("an sdk version is `<major>.<minor>`"))
    }
}

const MAX_VERSION_DIGITS: usize = 9;

fn parse(text: &str) -> Option<SdkVersion> {
    let (major, minor) = text.split_once('.')?;
    let number = |part: &str| {
        let plain = !part.is_empty()
            && part.len() <= MAX_VERSION_DIGITS
            && part.bytes().all(|b| b.is_ascii_digit());
        plain.then(|| part.parse().ok()).flatten()
    };
    Some(SdkVersion::new(number(major)?, number(minor)?))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn serializes_as_major_dot_minor() {
        assert_eq!(
            serde_json::to_string(&SDK_VERSION).unwrap(),
            "\"1.0\"",
            "serialize"
        );
        assert_eq!(
            serde_json::from_str::<SdkVersion>("\"1.12\"").unwrap(),
            SdkVersion::new(1, 12),
            "parse"
        );
        assert_eq!(SdkVersion::new(1, 2).to_string(), "1.2", "display");
        for bad in [
            "1",
            "1.",
            ".1",
            "a.b",
            "1.2.3",
            "-1.0",
            "1.+2",
            "1.0000000000",
        ] {
            assert!(
                serde_json::from_str::<SdkVersion>(&format!("\"{bad}\"")).is_err(),
                "{bad}"
            );
        }
    }
}
