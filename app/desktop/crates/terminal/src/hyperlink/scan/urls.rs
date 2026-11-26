//! Finds web addresses in text, and the address runs file-path detection skips.

use std::ops::Range;

const WWW: &str = "www.";
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

pub(crate) fn address_runs(text: &str) -> Vec<Range<usize>> {
    let mut runs = scheme_runs(text);
    runs.extend(www_runs(text));
    runs
}

fn scheme_runs(text: &str) -> Vec<Range<usize>> {
    text.match_indices("://")
        .map(|(index, _)| scheme_start(text, index)..run_end(text, index))
        .collect()
}

fn scheme_start(
    text: &str,
    separator: usize,
) -> usize {
    let before = text.get(..separator).unwrap_or_default();
    let scheme_len = before
        .chars()
        .rev()
        .take_while(|character| character.is_ascii_alphanumeric() || "+.-".contains(*character))
        .map(char::len_utf8)
        .sum::<usize>();
    separator - scheme_len
}

fn www_runs(text: &str) -> Vec<Range<usize>> {
    text.match_indices(WWW)
        .filter(|(index, _)| starts_word(text, *index))
        .map(|(index, _)| index..run_end(text, index))
        .collect()
}

fn starts_word(
    text: &str,
    index: usize,
) -> bool {
    let before = text.get(..index).and_then(|head| head.chars().next_back());
    before.is_none_or(|character| character.is_whitespace() || "\"'`([<".contains(character))
}

fn run_end(
    text: &str,
    from: usize,
) -> usize {
    let tail = text.get(from..).unwrap_or_default();
    from + tail.find(char::is_whitespace).unwrap_or(tail.len())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn texts(
        text: &str,
        ranges: Vec<Range<usize>>,
    ) -> Vec<&str> {
        ranges
            .into_iter()
            .filter_map(|range| text.get(range))
            .collect()
    }

    const CASES: &[(&str, &[&str])] = &[
        ("see (https://x.dev/a).", &["https://x.dev/a"]),
        (
            "go to http://localhost:3000/path?q=1, now",
            &["http://localhost:3000/path?q=1"],
        ),
        (
            "\"https://a.io/x\" and HTTPS://B.io",
            &["https://a.io/x", "HTTPS://B.io"],
        ),
        ("http:// alone", &[]),
        ("no links here", &[]),
    ];

    #[test]
    fn addresses_stop_before_trailing_punctuation_and_need_a_host() {
        for (text, expected) in CASES {
            assert_eq!(texts(text, find_urls(text)), *expected, "{text}");
        }
    }

    #[test]
    fn schemes_and_free_standing_www_start_address_runs() {
        let text = "ftp://h/a/b and www.x.com/y but bin/www.js";
        assert_eq!(
            texts(text, address_runs(text)),
            ["ftp://h/a/b", "www.x.com/y"]
        );
    }
}
