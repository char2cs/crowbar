use sha2::{Digest, Sha256};

pub(crate) const DIGEST_BYTES: usize = 32;
pub(crate) type CertificateDigest = [u8; DIGEST_BYTES];

const NIBBLE_BITS: u8 = 4;
const NIBBLE_MASK: u8 = 0x0f;
const HEX_RADIX: u32 = 16;
const DISPLAY_BYTES: usize = 12;
const GROUP_CHARS: usize = 4;

#[must_use]
pub(crate) fn certificate_digest(der: &[u8]) -> CertificateDigest {
    Sha256::digest(der).into()
}

#[must_use]
pub(crate) fn hex(bytes: &[u8]) -> String {
    bytes
        .iter()
        .flat_map(|byte| [byte >> NIBBLE_BITS, byte & NIBBLE_MASK])
        .filter_map(|nibble| char::from_digit(u32::from(nibble), HEX_RADIX))
        .collect()
}

/// A certificate's SHA-256 as grouped lowercase hex, byte-identical to the daemon's
/// `tlsid.FingerprintOf`, so `crowbar pair`'s output and the pairing UI compare directly.
#[must_use]
pub fn fingerprint_of(der: &[u8]) -> String {
    display_fingerprint(&certificate_digest(der))
}

pub(crate) fn display_fingerprint(digest: &CertificateDigest) -> String {
    let full = hex(digest.get(..DISPLAY_BYTES).unwrap_or_default());
    let groups: Vec<&str> = full
        .as_bytes()
        .chunks(GROUP_CHARS)
        .filter_map(|chunk| std::str::from_utf8(chunk).ok())
        .collect();
    format!("SHA256:{}", groups.join(" "))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_fingerprint_has_the_go_side_format() {
        let fingerprint = fingerprint_of(b"anything");
        let groups: Vec<&str> = fingerprint
            .strip_prefix("SHA256:")
            .unwrap()
            .split(' ')
            .collect();
        assert_eq!(groups.len(), 6);
        for group in groups {
            assert_eq!(group.len(), 4);
            assert!(
                group
                    .chars()
                    .all(|c| c.is_ascii_hexdigit() && !c.is_ascii_uppercase())
            );
        }
    }

    #[test]
    fn the_fingerprint_is_deterministic_and_input_sensitive() {
        assert_eq!(fingerprint_of(b"a"), fingerprint_of(b"a"));
        assert_ne!(fingerprint_of(b"a"), fingerprint_of(b"b"));
    }
}
