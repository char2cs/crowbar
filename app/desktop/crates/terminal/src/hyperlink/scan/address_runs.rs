use std::ops::Range;

const WWW: &str = "www.";

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

    #[test]
    fn schemes_and_free_standing_www_start_runs() {
        let text = "ftp://h/a/b and www.x.com/y but bin/www.js";
        let runs: Vec<&str> = address_runs(text)
            .into_iter()
            .filter_map(|range| text.get(range))
            .collect();
        assert_eq!(runs, ["ftp://h/a/b", "www.x.com/y"]);
    }
}
