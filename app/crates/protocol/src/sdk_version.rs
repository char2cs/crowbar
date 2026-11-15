use std::fmt;

use serde::{Deserialize, Deserializer, Serialize, Serializer};

use crate::error::{Error, Result};

/// The pane SDK version this build speaks.
pub const SDK_VERSION: SdkVersion = SdkVersion { major: 1, minor: 0 };

/// A pane SDK version, `<major>.<minor>`, as declared in extension manifests (`sdk: ^1`).
///
/// A major bump breaks frames; a minor bump only adds frame kinds, which an older peer never
/// receives because the shell sends only what [`SdkVersion::negotiate`] allows.
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

    /// The major version, which every frame carries as `sdk`.
    #[must_use]
    pub fn major(self) -> u32 {
        self.major
    }

    /// The minor version.
    #[must_use]
    pub fn minor(self) -> u32 {
        self.minor
    }

    /// The version both sides speak: the page's major must equal `shell`'s, and the minor is the
    /// lower of the two.
    ///
    /// # Errors
    /// [`Error::UnsupportedSdk`] when the majors differ.
    pub fn negotiate(
        self,
        shell: Self,
    ) -> Result<Self> {
        if self.major != shell.major {
            return Err(Error::unsupported_sdk(
                Some(u64::from(self.major)),
                shell.major,
            ));
        }
        Ok(Self {
            major: shell.major,
            minor: self.minor.min(shell.minor),
        })
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
    ) -> std::result::Result<S::Ok, S::Error> {
        serializer.collect_str(self)
    }
}

impl<'de> Deserialize<'de> for SdkVersion {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> std::result::Result<Self, D::Error> {
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
    fn negotiation_needs_equal_majors_and_takes_the_lower_minor() {
        let shell = SdkVersion::new(1, 2);
        assert_eq!(
            SdkVersion::new(1, 5).negotiate(shell).unwrap(),
            SdkVersion::new(1, 2),
            "newer page"
        );
        assert_eq!(
            SdkVersion::new(1, 0).negotiate(shell).unwrap(),
            SdkVersion::new(1, 0),
            "older page"
        );
        for major in [0, 2] {
            assert!(
                matches!(
                    SdkVersion::new(major, 0).negotiate(shell),
                    Err(Error::UnsupportedSdk { expected: 1, .. })
                ),
                "major {major}"
            );
        }
    }

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
        assert_eq!((SDK_VERSION.major(), SDK_VERSION.minor()), (1, 0), "parts");
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
