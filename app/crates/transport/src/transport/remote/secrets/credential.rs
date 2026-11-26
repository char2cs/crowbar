use std::fmt;

use serde_json::{Value, json};
use zeroize::Zeroizing;

use crate::transport::remote::tls::{CertificateDigest, hex};

const HEX_RADIX: u32 = 16;
const TOKEN_FIELD: &str = "token";
const PIN_FIELD: &str = "pin";

#[derive(Clone, PartialEq, Eq)]
pub(crate) struct Credential {
    token: Zeroizing<String>,
    pin: CertificateDigest,
}

impl fmt::Debug for Credential {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        f.debug_struct("Credential")
            .field("token", &"<redacted>")
            .field("pin", &hex(&self.pin))
            .finish()
    }
}

impl Credential {
    pub(crate) fn new(
        token: String,
        pin: CertificateDigest,
    ) -> Self {
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
        Zeroizing::new(
            json!({ TOKEN_FIELD: self.token.as_str(), PIN_FIELD: hex(&self.pin) }).to_string(),
        )
    }

    pub(crate) fn decode(stored: &str) -> Option<Self> {
        let value: Value = serde_json::from_str(stored).ok()?;
        let token = value
            .get(TOKEN_FIELD)?
            .as_str()
            .filter(|t| !t.trim().is_empty())?;
        let pin = parse_digest(value.get(PIN_FIELD)?.as_str()?)?;
        Some(Self::new(token.to_owned(), pin))
    }
}

fn parse_digest(text: &str) -> Option<CertificateDigest> {
    let mut digest = CertificateDigest::default();
    if text.len() != digest.len() * 2 || !text.is_ascii() {
        return None;
    }
    for (byte, pair) in digest.iter_mut().zip(text.as_bytes().chunks(2)) {
        *byte = u8::from_str_radix(std::str::from_utf8(pair).ok()?, HEX_RADIX).ok()?;
    }
    Some(digest)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_credential_round_trips_and_hides_its_token_from_debug() {
        let credential = Credential::new("secret-token".into(), [7; 32]);
        assert_eq!(
            Credential::decode(&credential.encode()),
            Some(credential.clone())
        );
        assert!(!format!("{credential:?}").contains("secret-token"));
    }

    #[test]
    fn anything_pairing_did_not_write_decodes_to_nothing() {
        let pin = "00".repeat(32);
        for stored in [
            "sometoken".to_owned(),
            String::new(),
            format!(r#"{{"token":"","pin":"{pin}"}}"#),
            format!(r#"{{"token":" \t","pin":"{pin}"}}"#),
            r#"{"token":"t","pin":"abcd"}"#.to_owned(),
            format!(r#"{{"token":"t","pin":"{}"}}"#, "zz".repeat(32)),
            format!(r#"{{"token":"t","pin":"{}é"}}"#, "0".repeat(63)),
            format!(r#"{{"pin":"{pin}"}}"#),
        ] {
            assert_eq!(Credential::decode(&stored), None, "{stored}");
        }
    }
}
