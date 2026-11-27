use base64::Engine as _;

use crate::live::DocumentBody;

const BASE64: &str = "base64";

pub(crate) fn text(body: &DocumentBody) -> String {
    if body.encoding != BASE64 {
        return body.content.clone();
    }
    base64::engine::general_purpose::STANDARD
        .decode(&body.content)
        .map_or_else(
            |_| body.content.clone(),
            |bytes| bytes.into_iter().map(char::from).collect(),
        )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn plain_content_stays_and_base64_bytes_read_as_latin_1() {
        let plain = DocumentBody {
            content: "héllo".into(),
            encoding: String::new(),
        };
        let binary = DocumentBody {
            content: "AP9B".into(),
            encoding: BASE64.into(),
        };
        assert_eq!(text(&plain), "héllo");
        assert_eq!(text(&binary), "\u{0}\u{ff}A");
    }
}
