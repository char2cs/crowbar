use crate::hyperlink::scan::path_match::PathMatch;

const PATH_PRECEDERS: &str = "\"'`([<";
const PATH_PUNCTUATION: &[u8] = b"_.@+~-";
const PREFIXES: [&[u8]; 4] = [b"~/", b"../", b"./", b"/"];

pub(crate) fn find_paths(text: &str) -> Vec<PathMatch> {
    let bytes = text.as_bytes();
    let from = |at: usize| (at..bytes.len()).find_map(|start| path_at(bytes, start));
    std::iter::successors(from(0), |previous| from(previous.span.end)).collect()
}

fn path_at(
    bytes: &[u8],
    start: usize,
) -> Option<PathMatch> {
    let boundary = start == 0 || bytes.get(start - 1).is_some_and(|byte| is_preceder(*byte));
    if !boundary {
        return None;
    }
    let path_end = path_end(bytes, start)?;
    let (end, line) = line_suffix(bytes, path_end);
    let trimmed = start + trim_dots(bytes.get(start..path_end)?);
    (trimmed > start).then_some(PathMatch {
        span: start..end,
        path_end: trimmed,
        line,
    })
}

fn is_preceder(byte: u8) -> bool {
    byte.is_ascii_whitespace() || PATH_PRECEDERS.as_bytes().contains(&byte)
}

fn is_path_byte(byte: u8) -> bool {
    byte.is_ascii_alphanumeric() || PATH_PUNCTUATION.contains(&byte)
}

fn skip_component(
    bytes: &[u8],
    from: usize,
) -> usize {
    let rest = bytes.get(from..).unwrap_or_default();
    from + rest.iter().take_while(|byte| is_path_byte(**byte)).count()
}

fn prefix_len(bytes: &[u8]) -> usize {
    PREFIXES
        .iter()
        .find(|prefix| bytes.starts_with(prefix))
        .map_or(0, |prefix| prefix.len())
}

fn path_end(
    bytes: &[u8],
    start: usize,
) -> Option<usize> {
    let rest = bytes.get(start..)?;
    nested_path_end(bytes, start + prefix_len(rest)).or_else(|| lone_rooted_end(bytes, start))
}

fn lone_rooted_end(
    bytes: &[u8],
    start: usize,
) -> Option<usize> {
    let rooted = bytes.get(start) == Some(&b'/');
    let end = skip_component(bytes, start + 1);
    (rooted && end > start + 1).then_some(end)
}

fn nested_path_end(
    bytes: &[u8],
    first: usize,
) -> Option<usize> {
    let mut end = skip_component(bytes, first);
    if end == first {
        return None;
    }
    let mut slashes = 0;
    while bytes.get(end) == Some(&b'/') && skip_component(bytes, end + 1) > end + 1 {
        end = skip_component(bytes, end + 1);
        slashes += 1;
    }
    let trailing = usize::from(bytes.get(end) == Some(&b'/'));
    (slashes > 0).then_some(end + trailing)
}

fn line_suffix(
    bytes: &[u8],
    from: usize,
) -> (usize, Option<u32>) {
    let first = number_after_colon(bytes, from);
    let second = first.and_then(|(next, _)| number_after_colon(bytes, next));
    let end = second.or(first).map_or(from, |(end, _)| end);
    (end, first.map(|(_, line)| line))
}

fn number_after_colon(
    bytes: &[u8],
    at: usize,
) -> Option<(usize, u32)> {
    if bytes.get(at) != Some(&b':') {
        return None;
    }
    let digits = bytes
        .get(at + 1..)?
        .iter()
        .take_while(|byte| byte.is_ascii_digit())
        .count();
    let text = std::str::from_utf8(bytes.get(at + 1..at + 1 + digits)?).ok()?;
    Some((at + 1 + digits, text.parse().ok()?))
}

fn trim_dots(path: &[u8]) -> usize {
    path.len() - path.iter().rev().take_while(|byte| **byte == b'.').count()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn paths(text: &str) -> Vec<(&str, Option<u32>)> {
        find_paths(text)
            .into_iter()
            .filter_map(|found| Some((text.get(found.span.start..found.path_end)?, found.line)))
            .collect()
    }

    #[test]
    fn paths_are_found_where_compilers_and_git_print_them() {
        let cases = [
            ("error at src/main.rs:12:4", vec![("src/main.rs", Some(12))]),
            (
                "modified: ./a/b.ts and /etc/hosts",
                vec![("./a/b.ts", None), ("/etc/hosts", None)],
            ),
            ("(see design/notes.md.)", vec![("design/notes.md", None)]),
            ("cd /tmp", vec![("/tmp", None)]),
            ("open README.md now", vec![]),
            ("a=b/c", vec![]),
        ];
        for (text, expected) in cases {
            assert_eq!(paths(text), expected, "{text}");
        }
    }
}
