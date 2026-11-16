//! Provider icons: SVG markup any paired client may upload, drawn only when it is static artwork.

const MAX_BYTES: usize = 64 * 1024;
const FORBIDDEN: [&str; 14] = [
    "<script",
    "<foreignobject",
    "<use",
    "<image",
    "<style",
    "<iframe",
    "<animate",
    "<set",
    "<a ",
    "<!doctype",
    "<!entity",
    "href",
    "javascript:",
    "<switch",
];

pub(crate) fn vetted(markup: &str) -> Option<&str> {
    let markup = markup.trim();
    let lower = markup.to_ascii_lowercase();
    let plain = lower.starts_with("<svg")
        && markup.len() <= MAX_BYTES
        && !FORBIDDEN.iter().any(|token| lower.contains(token));
    plain.then_some(markup)
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: &[(&str, bool)] = &[
        ("<svg viewBox=\"0 0 1 1\"><path d=\"M0 0\"/></svg>", true),
        (
            "  <svg xmlns:xlink=\"http://www.w3.org/1999/xlink\"/>",
            true,
        ),
        ("", false),
        ("<div/>", false),
        ("<svg><script>x</script></svg>", false),
        ("<svg><image href=\"file:///etc/passwd\"/></svg>", false),
        ("<svg><use xlink:href=\"#a\"/></svg>", false),
        ("<svg><style>*{}</style></svg>", false),
        ("<svg><SCRIPT/></svg>", false),
    ];

    #[test]
    fn only_static_svg_artwork_is_drawn() {
        for &(markup, drawn) in CASES {
            assert_eq!(vetted(markup).is_some(), drawn, "{markup}");
        }
    }

    #[test]
    fn oversized_markup_is_refused() {
        let big = format!("<svg>{}</svg>", "a".repeat(MAX_BYTES));
        assert!(vetted(&big).is_none());
    }
}
