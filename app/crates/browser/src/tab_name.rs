use url::Url;

const PLACEHOLDER: &str = "Browser";

/// The tab label: the page title when it has text, else the URL's host, else the URL, else
/// "Browser" for an empty address.
#[must_use]
pub fn tab_name(
    title: Option<&str>,
    url: &str,
) -> String {
    if let Some(title) = title.filter(|title| !title.trim().is_empty()) {
        return title.to_owned();
    }
    if url.is_empty() {
        return PLACEHOLDER.to_owned();
    }
    Url::parse(url)
        .ok()
        .and_then(|parsed| parsed.host_str().map(str::to_owned))
        .unwrap_or_else(|| url.to_owned())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_label_falls_back_from_title_to_host_to_placeholder() {
        let cases = [
            (Some("Docs"), "https://a.test/x", "Docs"),
            (Some("  "), "https://a.test:8080/x", "a.test"),
            (None, "https://a.test/x", "a.test"),
            (None, "about:blank", "about:blank"),
            (None, "", "Browser"),
        ];
        for (title, url, expected) in cases {
            assert_eq!(tab_name(title, url), expected, "{title:?} {url}");
        }
    }
}
