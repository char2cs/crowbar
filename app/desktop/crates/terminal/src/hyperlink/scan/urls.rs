//! Finds web addresses in text.

use std::ops::Range;

const SCHEMES: [&str; 2] = ["http://", "https://"];
const FORBIDDEN_INSIDE: &str = "\"'!*(){}|\\^<>`";
const FORBIDDEN_LAST: &str = "\"':,.!?{}|\\^~[]`()<>";

pub(crate) fn find_urls(text: &str) -> Vec<Range<usize>> {
    let lower = text.to_ascii_lowercase();
    let mut from = 0;
    std::iter::from_fn(|| next_url(text, &lower, &mut from)).collect()
}

fn next_url(
    text: &str,
    lower: &str,
    from: &mut usize,
) -> Option<Range<usize>> {
    let start = next_scheme(lower, *from)?;
    let end = url_end(text, start);
    *from = end.max(start + 1);
    address_range(text, start, end).or_else(|| next_url(text, lower, from))
}

fn address_range(
    text: &str,
    start: usize,
    end: usize,
) -> Option<Range<usize>> {
    has_host(text, start, end).then_some(start..end)
}

fn next_scheme(
    lower: &str,
    from: usize,
) -> Option<usize> {
    let rest = lower.get(from..)?;
    SCHEMES
        .iter()
        .filter_map(|scheme| rest.find(scheme))
        .min()
        .map(|offset| from + offset)
}

fn url_end(
    text: &str,
    start: usize,
) -> usize {
    let tail = text.get(start..).unwrap_or_default();
    let stop = tail
        .find(|character: char| character.is_whitespace() || FORBIDDEN_INSIDE.contains(character))
        .unwrap_or(tail.len());
    let body = tail.get(..stop).unwrap_or_default();
    start
        + body
            .trim_end_matches(|character| FORBIDDEN_LAST.contains(character))
            .len()
}

fn has_host(
    text: &str,
    start: usize,
    end: usize,
) -> bool {
    let Some(address) = text.get(start..end) else {
        return false;
    };
    let after_scheme = address.split_once("://").map_or("", |(_, rest)| rest);
    after_scheme
        .chars()
        .next()
        .is_some_and(|first| first.is_alphanumeric() || first == '[')
}

#[cfg(test)]
mod tests {
    use super::*;

    fn urls(text: &str) -> Vec<&str> {
        find_urls(text)
            .into_iter()
            .filter_map(|range| text.get(range))
            .collect()
    }

    #[test]
    fn addresses_stop_before_trailing_punctuation_and_brackets() {
        assert_eq!(urls("see (https://x.dev/a)."), ["https://x.dev/a"]);
        assert_eq!(
            urls("go to http://localhost:3000/path?q=1, now"),
            ["http://localhost:3000/path?q=1"]
        );
        assert_eq!(
            urls("\"https://a.io/x\" and HTTPS://B.io"),
            ["https://a.io/x", "HTTPS://B.io"]
        );
    }

    #[test]
    fn a_scheme_without_a_host_is_not_a_link() {
        assert_eq!(urls("http:// alone").len(), 0, "no host");
        assert_eq!(urls("no links here").len(), 0, "no scheme");
    }
}
