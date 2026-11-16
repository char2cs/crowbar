//! Turns key presses into the bytes a PTY expects.

use crate::cursor_keys::CursorKeys;
use crate::key_input::KeyInput;
use crate::modifiers::Modifiers;
use crate::named_keys::named_sequence;

const ESC: u8 = 0x1b;
const NUL: u8 = 0;
const FILE_SEPARATOR: u8 = 0x1c;
const GROUP_SEPARATOR: u8 = 0x1d;
const RECORD_SEPARATOR: u8 = 0x1e;
const UNIT_SEPARATOR: u8 = 0x1f;
const CSI_U_SHIFT_ENTER: &[u8] = b"\x1b[13;2u";
const CSI_U_ALT_ENTER: &[u8] = b"\x1b[13;3u";
const CARRIAGE_RETURN: &[u8] = b"\r";
const CONTROL_SYMBOLS: [(char, u8); 14] = [
    ('@', NUL),
    (' ', NUL),
    ('2', NUL),
    ('[', ESC),
    ('3', ESC),
    ('\\', FILE_SEPARATOR),
    ('4', FILE_SEPARATOR),
    (']', GROUP_SEPARATOR),
    ('5', GROUP_SEPARATOR),
    ('^', RECORD_SEPARATOR),
    ('6', RECORD_SEPARATOR),
    ('_', UNIT_SEPARATOR),
    ('/', UNIT_SEPARATOR),
    ('7', UNIT_SEPARATOR),
];

pub(crate) fn encode_key(
    input: &KeyInput,
    cursor_keys: CursorKeys,
) -> Option<Vec<u8>> {
    let modifiers = input.modifiers;
    if modifiers.contains(Modifiers::PLATFORM) {
        return None;
    }
    if input.key == "enter" {
        return Some(enter_sequence(modifiers));
    }
    if let Some(named) = named_sequence(&input.key, modifiers, cursor_keys) {
        return Some(named);
    }
    if modifiers.contains(Modifiers::CONTROL) {
        return control_sequence(input);
    }
    printable(input)
}

fn enter_sequence(modifiers: Modifiers) -> Vec<u8> {
    if modifiers.contains(Modifiers::CONTROL) {
        return CARRIAGE_RETURN.to_vec();
    }
    if modifiers.contains(Modifiers::SHIFT) {
        return CSI_U_SHIFT_ENTER.to_vec();
    }
    if modifiers.contains(Modifiers::ALT) {
        return CSI_U_ALT_ENTER.to_vec();
    }
    CARRIAGE_RETURN.to_vec()
}

fn control_sequence(input: &KeyInput) -> Option<Vec<u8>> {
    let byte = control_byte(&input.key)?;
    if input.modifiers.contains(Modifiers::ALT) {
        return Some(vec![ESC, byte]);
    }
    Some(vec![byte])
}

fn control_byte(key: &str) -> Option<u8> {
    if key == "space" {
        return Some(NUL);
    }
    let mut characters = key.chars();
    let only = characters.next().filter(|_| characters.next().is_none())?;
    let letter = u8::try_from(only).ok().filter(u8::is_ascii_lowercase);
    if let Some(letter) = letter {
        return Some(letter - b'a' + 1);
    }
    CONTROL_SYMBOLS
        .iter()
        .find(|(symbol, _)| *symbol == only)
        .map(|(_, byte)| *byte)
}

fn printable(input: &KeyInput) -> Option<Vec<u8>> {
    let text = input.text.as_deref().filter(|text| !text.is_empty())?;
    let prefix = input.modifiers.contains(Modifiers::ALT).then_some(ESC);
    let mut bytes: Vec<u8> = prefix.into_iter().collect();
    bytes.extend_from_slice(text.as_bytes());
    Some(bytes)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn press(
        key: &str,
        text: Option<&str>,
        modifiers: Modifiers,
    ) -> KeyInput {
        KeyInput {
            key: key.into(),
            text: text.map(str::to_owned),
            modifiers,
        }
    }

    #[test]
    fn keys_encode_to_their_bytes() {
        let none = Modifiers::NONE;
        let cases = [
            (press("a", Some("a"), none), Some(b"a".to_vec())),
            (press("a", Some("A"), Modifiers::SHIFT), Some(b"A".to_vec())),
            (press("c", Some("c"), Modifiers::CONTROL), Some(vec![3])),
            (press("space", Some(" "), Modifiers::CONTROL), Some(vec![0])),
            (
                press("b", Some("b"), Modifiers::ALT),
                Some(b"\x1bb".to_vec()),
            ),
            (
                press("d", Some("d"), Modifiers::CONTROL.with(Modifiers::ALT)),
                Some(vec![0x1b, 4]),
            ),
            (press("enter", None, none), Some(b"\r".to_vec())),
            (
                press("enter", None, Modifiers::SHIFT),
                Some(b"\x1b[13;2u".to_vec()),
            ),
            (
                press("enter", None, Modifiers::ALT),
                Some(b"\x1b[13;3u".to_vec()),
            ),
            (press("space", Some(" "), none), Some(b" ".to_vec())),
            (press("c", Some("c"), Modifiers::PLATFORM), None),
            (press("f13", None, none), None),
            (press("é", Some("é"), none), Some("é".as_bytes().to_vec())),
            (press("]", Some("]"), Modifiers::CONTROL), Some(vec![0x1d])),
            (press("up", None, none), Some(b"\x1b[A".to_vec())),
        ];
        for (input, expected) in cases {
            assert_eq!(
                encode_key(&input, CursorKeys::Normal),
                expected,
                "{input:?}"
            );
        }
    }

    #[test]
    fn application_cursor_mode_changes_arrows() {
        let up = press("up", None, Modifiers::NONE);
        assert_eq!(
            encode_key(&up, CursorKeys::Application),
            Some(b"\x1bOA".to_vec())
        );
    }
}
