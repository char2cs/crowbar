//! An env file with every value emptied: the keys and the comments stay.

const EXPORT: &str = "export";
const NEWLINE: char = '\n';

fn value_after_name(text: &str) -> Option<&str> {
    let mut characters = text.chars();
    let first = characters
        .next()
        .filter(|found| found.is_ascii_alphabetic() || *found == '_')?;
    let tail = characters
        .take_while(|found| found.is_ascii_alphanumeric() || *found == '_')
        .count();
    let after_name = text.get(first.len_utf8() + tail..)?;
    let after_sign = after_name.trim_start().strip_prefix('=')?;
    Some(after_sign.trim_start())
}

fn value_of(line: &str) -> Option<&str> {
    let start = line.trim_start();
    let exported = start
        .strip_prefix(EXPORT)
        .filter(|after| after.starts_with(char::is_whitespace))
        .and_then(|after| value_after_name(after.trim_start()));
    exported.or_else(|| value_after_name(start))
}

fn advance(
    (quote, escaped): (Option<char>, bool),
    found: char,
) -> (Option<char>, bool) {
    if escaped {
        return (quote, false);
    }
    if matches!((found, quote), ('\\', Some('"'))) {
        return (quote, true);
    }
    let toggles = quote.map_or(matches!(found, '\'' | '"'), |open| open == found);
    if toggles {
        return (quote.map_or(Some(found), |_| None), false);
    }
    (quote, false)
}

fn comment_start(value: &str) -> Option<usize> {
    let mut state = (None, false);
    let mut previous: Option<char> = None;
    value.char_indices().find_map(|(index, found)| {
        let spaced = previous.is_none_or(char::is_whitespace);
        let opens = matches!((found, state.0, spaced), ('#', None, true));
        let start = opens.then(|| index - previous.map_or(0, char::len_utf8));
        state = advance(state, found);
        previous = Some(found);
        start
    })
}

fn blank_value(line: &str) -> String {
    let skipped = line.trim().is_empty() || line.trim_start().starts_with('#');
    let Some(value) = value_of(line).filter(|_| !skipped) else {
        return line.to_owned();
    };
    let comment = comment_start(value).and_then(|start| value.get(start..));
    let prefix = line.get(..line.len() - value.len()).unwrap_or_default();
    format!("{prefix}{}", comment.unwrap_or_default())
}

pub(crate) fn template_of(content: &str) -> String {
    let normalized = content.replace("\r\n", "\n");
    let trailing = normalized.ends_with(NEWLINE);
    let body = normalized.strip_suffix(NEWLINE).unwrap_or(&normalized);
    let lines: Vec<String> = body.split(NEWLINE).map(blank_value).collect();
    let template = lines.join("\n");
    if trailing {
        return format!("{template}\n");
    }
    template
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(&str, &str, &str); 14] = [
        ("value emptied", "API_KEY=abc123", "API_KEY="),
        ("export kept", "export TOKEN=xyz", "export TOKEN="),
        ("spaces around sign", "NAME = spaced value", "NAME = "),
        ("comment line kept", "# a note\nA=1\n", "# a note\nA=\n"),
        ("blank line kept", "A=1\n\nB=2\n", "A=\n\nB=\n"),
        ("inline comment kept", "A=1 # why", "A= # why"),
        ("hash inside quotes", "A=\"x # y\"", "A="),
        ("hash after quoted", "A=\"x\" # note", "A= # note"),
        ("hash without space", "A=b#c", "A="),
        ("not an assignment", "just text", "just text"),
        ("crlf becomes lf", "A=1\r\nB=2\r\n", "A=\nB=\n"),
        ("no trailing newline", "A=1\nB=2", "A=\nB="),
        ("export as a name", "export = 1", "export = "),
        ("empty file", "", ""),
    ];

    #[test]
    fn values_are_emptied_and_everything_else_stays() {
        for (name, content, expected) in CASES {
            assert_eq!(template_of(content), expected, "{name}");
        }
    }

    #[test]
    fn an_escaped_quote_does_not_end_the_value() {
        assert_eq!(template_of(r#"A="x \" # y" # z"#), "A= # z");
    }
}
