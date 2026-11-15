use crate::tls_pin::{hex, CertificateDigest};

const HEX_RADIX: u32 = 16;

#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) struct Credential {
    pub(crate) token: String,
    pub(crate) pin: CertificateDigest,
}

impl Credential {
    pub(crate) fn encode(&self) -> String {
        serde_json::json!({ "token": self.token, "pin": hex(&self.pin) }).to_string()
    }

    pub(crate) fn decode(stored: &str) -> Option<Self> {
        let value: serde_json::Value = serde_json::from_str(stored).ok()?;
        let token = value
            .get("token")
            .and_then(serde_json::Value::as_str)
            .filter(|t| !t.trim().is_empty())?;
        Some(Self {
            token: token.to_owned(),
            pin: parse_digest(value.get("pin").and_then(serde_json::Value::as_str)?)?,
        })
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

    fn credential() -> Credential {
        Credential {
            token: "sometoken".into(),
            pin: std::array::from_fn(|i| u8::try_from(i).unwrap() * 7),
        }
    }

    #[test]
    fn a_credential_reads_back_as_written() {
        let credential = credential();
        assert_eq!(Credential::decode(&credential.encode()), Some(credential));
    }

    #[test]
    fn nothing_else_reads_as_a_credential() {
        let pin = hex(&credential().pin);
        let cases = [
            "sometoken".to_owned(),
            serde_json::json!({ "token": "sometoken" }).to_string(),
            serde_json::json!({ "token": " ", "pin": pin }).to_string(),
            serde_json::json!({ "token": "sometoken", "pin": pin.chars().skip(2).collect::<String>() }).to_string(),
            serde_json::json!({ "token": "sometoken", "pin": pin.replace('0', "g") }).to_string(),
        ];
        for stored in cases {
            assert_eq!(Credential::decode(&stored), None, "{stored}");
        }
    }
}
