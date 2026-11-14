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
    pub fn new(major: u32, minor: u32) -> Self {
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
    pub fn negotiate(self, shell: Self) -> Result<Self> {
        if self.major != shell.major {
            return Err(Error::UnsupportedSdk {
                found: Some(u64::from(self.major)),
                expected: shell.major,
            });
        }
        Ok(Self {
            major: shell.major,
            minor: self.minor.min(shell.minor),
        })
    }
}

impl fmt::Display for SdkVersion {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}.{}", self.major, self.minor)
    }
}

impl Serialize for SdkVersion {
    fn serialize<S: Serializer>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error> {
        serializer.collect_str(self)
    }
}

impl<'de> Deserialize<'de> for SdkVersion {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> std::result::Result<Self, D::Error> {
        let text = String::deserialize(deserializer)?;
        parse(&text).ok_or_else(|| serde::de::Error::custom("an sdk version is `<major>.<minor>`"))
    }
}

fn parse(text: &str) -> Option<SdkVersion> {
    let (major, minor) = text.split_once('.')?;
    let number = |part: &str| {
        let plain = !part.is_empty() && part.len() <= 9 && part.bytes().all(|b| b.is_ascii_digit());
        plain.then(|| part.parse().ok()).flatten()
    };
    Some(SdkVersion::new(number(major)?, number(minor)?))
}

#[cfg(test)]
#[path = "sdk_version_tests.rs"]
mod tests;
