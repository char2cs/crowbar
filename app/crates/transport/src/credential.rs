use std::fmt;

use serde_json::{Value, json};
use zeroize::Zeroizing;

use crate::fingerprint::{CertificateDigest, hex};

#[derive(Clone, PartialEq, Eq)]
pub(crate) struct Credential {
    token: Zeroizing<String>,
    pin: CertificateDigest,
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
    pub(crate) fn new(token: String, pin: CertificateDigest) -> Self {
        Self {
            token: Zeroizing::new(token),
            pin,
        }
    }

    pub(crate) fn token(&self) -> &str {
        &self.token
    }

    pub(crate) fn pin(&self) -> CertificateDigest {
        self.pin
    }

    pub(crate) fn encode(&self) -> Zeroizing<String> {
        Zeroizing::new(json!({ "token": self.token.as_str(), "pin": hex(&self.pin) }).to_string())
    }

    pub(crate) fn decode(stored: &str) -> Option<Self> {
        let value: Value = serde_json::from_str(stored).ok()?;
        let token = value
            .get("token")?
            .as_str()
            .filter(|t| !t.trim().is_empty())?;
        let pin = parse_digest(value.get("pin")?.as_str()?)?;
        Some(Self::new(token.to_owned(), pin))
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
