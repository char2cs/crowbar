use crate::error::{Error, Result};
use crate::string_id::string_id;

const KIND: &str = "external url";
const MAX_URL_BYTES: usize = 2048;
const SCHEMES: &[&str] = &["http://", "https://", "mailto:", "tel:"];

fn has_allowed_scheme(value: &str) -> bool {
    let lowered = value.to_ascii_lowercase();
    SCHEMES.iter().any(|scheme| lowered.starts_with(scheme))
}

const SCHEME_REASON: &str = "must start with http://, https://, mailto: or tel:";

fn validate(value: &str) -> Result<()> {
    if value.is_empty() {
        return Err(Error::EmptyId { kind: KIND });
    }
    if value.len() > MAX_URL_BYTES {
        return Err(Error::too_long(KIND, MAX_URL_BYTES));
    }
    if let Some(found) = value.chars().find(|c| !c.is_ascii_graphic()) {
        return Err(Error::InvalidIdCharacter { kind: KIND, found });
    }
    if !has_allowed_scheme(value) {
        return Err(Error::malformed(KIND, SCHEME_REASON));
    }
    Ok(())
}

string_id!(
    /// A link a pane asks the shell to open in the system's default application: an `http`,
    /// `https`, `mailto` or `tel` URL of printable ASCII, at most 2048 bytes.
    ExternalUrl,
    validate
);

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accepts_web_mail_and_phone_links_only() {
        for ok in [
            "https://example.com/a?b=1",
            "HTTP://x.test",
            "mailto:a@b.c",
            "tel:+491",
        ] {
            assert!(ExternalUrl::new(ok).is_ok(), "{ok}");
        }
        let long = format!("https://a.test/{}", "x".repeat(MAX_URL_BYTES));
        for bad in [
            "",
            "file:///etc/passwd",
            "javascript:alert(1)",
            "-oProxy",
            "https://a b",
            "https://\u{e9}",
            long.as_str(),
        ] {
            assert!(ExternalUrl::new(bad).is_err(), "{bad:?}");
        }
    }
}
