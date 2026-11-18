//! Pasted text on its way to the program.

use crate::input::paste_mode::PasteMode;

const BRACKET_START: &str = "\x1b[200~";
const BRACKET_END: &str = "\x1b[201~";

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

#[cfg(test)]
mod tests {
    use super::*;

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
