use sha2::{Digest, Sha256};
use std::fmt::Write;

/// A certificate's whole SHA-256.
pub type CertificateDigest = [u8; 32];

const DISPLAY_BYTES: usize = 12;
const GROUP_CHARS: usize = 4;

/// The SHA-256 of a DER certificate.
#[must_use]
pub fn certificate_digest(der: &[u8]) -> CertificateDigest {
    Sha256::digest(der).into()
}

/// Lowercase hex of `bytes`.
#[must_use]
pub fn hex(bytes: &[u8]) -> String {
    bytes.iter().fold(String::new(), |mut text, byte| {
        write!(text, "{byte:02x}").ok();
        text
    })
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
#[path = "fingerprint_tests.rs"]
mod tests;
