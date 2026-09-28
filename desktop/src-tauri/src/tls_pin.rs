//! Fingerprint pinning for remote daemon connections: instead of validating a
//! certificate chain against a CA (there is none — every daemon's identity is
//! self-signed, `api/internal/core/tlsid`), a remote host's certificate is
//! trusted exactly when its fingerprint matches the one the user confirmed
//! during pairing.

use sha2::{Digest, Sha256};

/// Renders a certificate's SHA-256 as grouped lowercase hex — byte-identical
/// in format to the Go daemon's `tlsid.FingerprintOf`, so the string a human
/// reads from `crowbar pair` on the daemon's own host and the string shown
/// here during pairing are directly comparable.
#[allow(dead_code)] // dead_code: wired into the PinnedVerifier landing in this file next
pub fn fingerprint_of(der: &[u8]) -> String {
    let hash = Sha256::digest(der);
    let full: String = hash.iter().map(|b| format!("{b:02x}")).collect();
    let groups: Vec<&str> = (0..6).map(|i| &full[i * 4..(i + 1) * 4]).collect();
    format!("SHA256:{}", groups.join(" "))
}

/// Whether `der`'s fingerprint matches `expected` (as produced by
/// `fingerprint_of` or by the Go daemon's `tlsid.FingerprintOf`).
#[allow(dead_code)] // dead_code: wired into the PinnedVerifier landing in this file next
pub fn fingerprint_matches(der: &[u8], expected: &str) -> bool {
    fingerprint_of(der) == expected
}

#[cfg(test)]
mod tests {
    use super::fingerprint_of;

    #[test]
    fn fingerprint_of_is_deterministic() {
        let der = b"some fake certificate bytes for a unit test";
        assert_eq!(fingerprint_of(der), fingerprint_of(der));
    }

    #[test]
    fn fingerprint_of_has_the_go_side_format() {
        // Format contract with api/internal/core/tlsid/tlsid.go's FingerprintOf:
        // "SHA256:" + six groups of 4 lowercase hex chars, space-separated —
        // the first 24 hex chars (12 bytes) of the SHA-256 digest, truncated
        // the same way on both sides so a human can visually compare them.
        let fp = fingerprint_of(b"anything");
        assert!(fp.starts_with("SHA256:"));
        let groups: Vec<&str> = fp["SHA256:".len()..].trim().split(' ').collect();
        assert_eq!(groups.len(), 6, "expected 6 groups, got {:?}", groups);
        for g in groups {
            assert_eq!(
                g.len(),
                4,
                "expected each group to be 4 hex chars, got {:?}",
                g
            );
            assert!(g.chars().all(|c| c.is_ascii_hexdigit()));
        }
    }

    #[test]
    fn fingerprint_of_distinguishes_different_input() {
        assert_ne!(fingerprint_of(b"a"), fingerprint_of(b"b"));
    }
}
