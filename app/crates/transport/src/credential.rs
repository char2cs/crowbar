use std::fmt;

use serde_json::{Value, json};

use crate::fingerprint::{CertificateDigest, hex};

/// A host's token and the certificate it is bound to, kept in one keychain entry so nothing the
/// webview or the host config can write moves the token to another identity.
#[derive(Clone, PartialEq, Eq)]
pub struct Credential {
    /// The bearer token the host issued at pairing.
    pub token: String,
    /// The whole digest of the certificate pairing verified.
    pub pin: CertificateDigest,
}

impl fmt::Debug for Credential {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("Credential")
            .field("token", &"<redacted>")
            .field("pin", &hex(&self.pin))
            .finish()
    }
}

impl Credential {
    /// The keychain entry text for this credential.
    #[must_use]
    pub fn encode(&self) -> String {
        json!({ "token": self.token, "pin": hex(&self.pin) }).to_string()
    }

    /// `None` for anything pairing did not write, including a blank token.
    #[must_use]
    pub fn decode(stored: &str) -> Option<Self> {
        let value: Value = serde_json::from_str(stored).ok()?;
        let token = value
            .get("token")?
            .as_str()
            .filter(|t| !t.trim().is_empty())?;
        let pin = parse_digest(value.get("pin")?.as_str()?)?;
        Some(Self {
            token: token.to_owned(),
            pin,
        })
    }
}

fn parse_digest(text: &str) -> Option<CertificateDigest> {
    let mut digest = CertificateDigest::default();
    if text.len() != digest.len() * 2 || !text.is_ascii() {
        return None;
    }
    for (byte, pair) in digest.iter_mut().zip(text.as_bytes().chunks(2)) {
        *byte = u8::from_str_radix(std::str::from_utf8(pair).ok()?, 16).ok()?;
    }
    Some(digest)
}

#[cfg(test)]
#[path = "credential_tests.rs"]
mod tests;
