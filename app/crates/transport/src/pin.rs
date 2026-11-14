use std::fmt;

use crate::fingerprint::{
    CertificateDigest, certificate_digest, display_fingerprint, fingerprint_of,
};

/// The one certificate a pinned dial trusts.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Pin {
    /// The fingerprint a human confirmed against `crowbar pair`. Truncated, so it pins only
    /// the pairing dial, which captures the full digest.
    Confirmed(String),
    /// The whole digest of the certificate pairing verified: every later dial.
    Paired(CertificateDigest),
}

impl Pin {
    pub(crate) fn matches(&self, der: &[u8]) -> bool {
        match self {
            Self::Confirmed(fingerprint) => fingerprint_of(der) == *fingerprint,
            Self::Paired(digest) => certificate_digest(der) == *digest,
        }
    }
}

impl fmt::Display for Pin {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Confirmed(fingerprint) => f.write_str(fingerprint),
            Self::Paired(digest) => f.write_str(&display_fingerprint(digest)),
        }
    }
}

#[cfg(test)]
#[path = "pin_tests.rs"]
mod tests;
