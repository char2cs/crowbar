use std::fmt;

use crate::fingerprint::{
    CertificateDigest, certificate_digest, display_fingerprint, fingerprint_of,
};

#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) enum Pin {
    /// The fingerprint a human confirmed against `crowbar pair`. Truncated, so it pins only
    /// the pairing dial, which captures the full digest.
    Confirmed(String),
    /// The whole digest of the certificate pairing verified: every later dial.
    Paired(CertificateDigest),
}

impl Pin {
    pub(crate) fn matches(
        &self,
        der: &[u8],
    ) -> bool {
        match self {
            Self::Confirmed(fingerprint) => fingerprint_of(der) == *fingerprint,
            Self::Paired(digest) => certificate_digest(der) == *digest,
        }
    }
}

impl fmt::Display for Pin {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Confirmed(fingerprint) => f.write_str(fingerprint),
            Self::Paired(digest) => f.write_str(&display_fingerprint(digest)),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const DER: &[u8] = b"not a real certificate, only bytes to hash";

    #[test]
    fn a_confirmed_pin_matches_the_displayed_fingerprint_of_the_certificate() {
        let pin = Pin::Confirmed(fingerprint_of(DER));
        assert!(pin.matches(DER));
        assert!(!pin.matches(b"another certificate"));
    }

    #[test]
    fn a_paired_pin_matches_the_whole_digest() {
        let pin = Pin::Paired(certificate_digest(DER));
        assert!(pin.matches(DER));
        assert!(!pin.matches(b"another certificate"));
    }

    #[test]
    fn both_pins_display_as_the_same_fingerprint() {
        let confirmed = Pin::Confirmed(fingerprint_of(DER));
        let paired = Pin::Paired(certificate_digest(DER));
        assert_eq!(confirmed.to_string(), paired.to_string());
    }
}
