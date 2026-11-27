use percent_encoding::{AsciiSet, NON_ALPHANUMERIC, utf8_percent_encode};

use crate::address::SearchEngine;

pub const BROWSER_PANE_TYPE: &str = "core.browser";

const COMPONENT: &AsciiSet = &NON_ALPHANUMERIC
    .remove(b'-')
    .remove(b'_')
    .remove(b'.')
    .remove(b'!')
    .remove(b'~')
    .remove(b'*')
    .remove(b'\'')
    .remove(b'(')
    .remove(b')');

#[must_use]
pub fn resolve_address(
    input: &str,
    engine: &SearchEngine,
) -> Option<String> {
    let text = input.trim();
    if text.is_empty() {
        return None;
    }
    if has_web_scheme(text) {
        return Some(text.to_owned());
    }
    if is_local_host(text) {
        return Some(format!("http://{text}"));
    }
    if is_dotted_host(text) {
        return Some(format!("https://{text}"));
    }
    let query = utf8_percent_encode(text, COMPONENT).to_string();
    Some(engine.template().replace("%s", &query))
}

fn has_web_scheme(text: &str) -> bool {
    let lowered = text.to_ascii_lowercase();
    lowered.starts_with("http://") || lowered.starts_with("https://")
}

fn is_local_host(text: &str) -> bool {
    let lowered = text.to_ascii_lowercase();
    ["localhost", "127.0.0.1"].iter().any(|host| {
        lowered
            .strip_prefix(host)
            .is_some_and(|rest| rest.is_empty() || rest.starts_with([':', '/']))
    })
}

fn is_dotted_host(text: &str) -> bool {
    if text.contains(char::is_whitespace) {
        return false;
    }
    let host_end = text.find(['/', '?', '#']).unwrap_or(text.len());
    let authority = text.get(..host_end).unwrap_or(text);
    let (host, port) = authority.split_once(':').unwrap_or((authority, ""));
    let labels: Vec<&str> = host.split('.').collect();
    let port_ok = port.chars().all(|c| c.is_ascii_digit());
    labels.len() > 1 && port_ok && labels.iter().all(|label| is_label(label))
}

fn is_label(label: &str) -> bool {
    !label.is_empty()
        && label
            .chars()
            .all(|c| c.is_alphanumeric() || c == '_' || c == '-')
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: &[(&str, Option<&str>)] = &[
        ("", None),
        ("   ", None),
        ("https://a.test/x", Some("https://a.test/x")),
        ("HTTP://A.test", Some("HTTP://A.test")),
        ("localhost:3000/app", Some("http://localhost:3000/app")),
        ("127.0.0.1", Some("http://127.0.0.1")),
        ("localhost.evil.com", Some("https://localhost.evil.com")),
        ("example.com", Some("https://example.com")),
        ("docs.rs/url?x=1#top", Some("https://docs.rs/url?x=1#top")),
        ("example.com:8080/a", Some("https://example.com:8080/a")),
        ("rust", Some("https://duckduckgo.com/?q=rust")),
        (
            "how to cook rice",
            Some("https://duckduckgo.com/?q=how%20to%20cook%20rice"),
        ),
        ("a.b c", Some("https://duckduckgo.com/?q=a.b%20c")),
        (
            "c++ & rust",
            Some("https://duckduckgo.com/?q=c%2B%2B%20%26%20rust"),
        ),
    ];

    #[test]
    fn typed_text_becomes_a_navigation() {
        for (input, expected) in CASES {
            assert_eq!(
                resolve_address(input, &SearchEngine::DuckDuckGo).as_deref(),
                *expected,
                "{input:?}"
            );
        }
    }
}
