use serde::Deserialize;

use crate::store::pairing::local_dates;

const CODE_GROUP: usize = 3;

#[derive(Debug, Clone, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Offer {
    pub code: String,
    #[serde(rename = "expiresAt")]
    pub until: String,
    pub addresses: Vec<String>,
    #[serde(rename = "serverFingerprint")]
    pub fingerprint: String,
}

impl Offer {
    pub(crate) fn shown(self) -> Self {
        let split = self.code.len().min(CODE_GROUP);
        let (head, tail) = self.code.split_at(split);
        Self {
            code: format!("{head} {tail}"),
            until: local_dates::time(&self.until).unwrap_or_else(|| self.until.clone()),
            ..self
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_daemon_s_code_is_grouped_in_threes_and_its_expiry_kept_when_unreadable() {
        let minted = serde_json::json!({
            "code": "123456",
            "serverFingerprint": "SHA256:ab",
            "addresses": [],
            "expiresAt": "soon",
        });
        let offer = serde_json::from_value::<Offer>(minted).unwrap().shown();
        assert_eq!(
            (offer.code.as_str(), offer.until.as_str()),
            ("123 456", "soon")
        );
    }
}
