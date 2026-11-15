use std::fmt;

use super::digest::{certificate_digest, display_fingerprint, fingerprint_of, CertificateDigest};

#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) enum Pin {
    Confirmed(String),
    Paired(CertificateDigest),
}

impl Pin {
    pub(super) fn matches(
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

    const DER: &[u8] = b"certificate bytes";

    #[test]
    fn a_pin_matches_only_its_own_certificate() {
        let mut flipped = certificate_digest(DER);
        flipped[31] ^= 1;
        let cases = [
            (Pin::Confirmed(fingerprint_of(DER)), true),
            (
                Pin::Confirmed("SHA256:0000 0000 0000 0000 0000 0000".into()),
                false,
            ),
            (Pin::Paired(certificate_digest(DER)), true),
            (Pin::Paired(flipped), false),
        ];
        for (pin, matches) in cases {
            assert_eq!(pin.matches(DER), matches, "{pin}");
        }
    }

    #[test]
    fn a_pin_displays_the_human_fingerprint_in_either_form() {
        let confirmed = Pin::Confirmed(fingerprint_of(DER));
        let paired = Pin::Paired(certificate_digest(DER));
        assert_eq!(confirmed.to_string(), fingerprint_of(DER));
        assert_eq!(paired.to_string(), fingerprint_of(DER));
    }
}
