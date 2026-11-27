use std::fmt;

use crate::transport::remote::tls::fingerprint::{
    CertificateDigest, certificate_digest, display_fingerprint, fingerprint_of,
};

#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) enum Pin {
    Confirmed(String),
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

    type Make = fn(&[u8]) -> Pin;

    const CASES: [Make; 2] = [
        |der| Pin::Confirmed(fingerprint_of(der)),
        |der| Pin::Paired(certificate_digest(der)),
    ];

    #[test]
    fn a_pin_matches_the_certificate_it_was_made_from_and_no_other() {
        for make in CASES {
            let pin = make(DER);
            assert!(pin.matches(DER));
            assert!(!pin.matches(b"another certificate"));
        }
    }

    #[test]
    fn both_pins_display_as_the_same_fingerprint() {
        let confirmed = Pin::Confirmed(fingerprint_of(DER));
        let paired = Pin::Paired(certificate_digest(DER));
        assert_eq!(confirmed.to_string(), paired.to_string());
    }
}
