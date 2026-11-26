//! Turns key presses into the bytes a PTY expects.

use crate::input::keyboard::cursor_keys::CursorKeys;
use crate::input::keyboard::key_input::KeyInput;
use crate::input::modifiers::Modifiers;

const ESC: u8 = 0x1b;
const NUL: u8 = 0;
const FILE_SEPARATOR: u8 = 0x1c;
const GROUP_SEPARATOR: u8 = 0x1d;
const RECORD_SEPARATOR: u8 = 0x1e;
const UNIT_SEPARATOR: u8 = 0x1f;
const CSI_U_SHIFT_ENTER: &[u8] = b"\x1b[13;2u";
const CSI_U_ALT_ENTER: &[u8] = b"\x1b[13;3u";
const CARRIAGE_RETURN: &[u8] = b"\r";
const ESC_TEXT: &str = "\x1b";
const FUNCTION_TILDE_CODES: [(&str, &str); 8] = [
    ("f5", "15"),
    ("f6", "17"),
    ("f7", "18"),
    ("f8", "19"),
    ("f9", "20"),
    ("f10", "21"),
    ("f11", "23"),
    ("f12", "24"),
];
const TILDE_KEYS: [(&str, &str); 4] = [
    ("pageup", "5"),
    ("pagedown", "6"),
    ("delete", "3"),
    ("insert", "2"),
];
const SS3_FUNCTION_KEYS: [(&str, char); 4] = [("f1", 'P'), ("f2", 'Q'), ("f3", 'R'), ("f4", 'S')];
const CURSOR_KEYS: [(&str, char); 6] = [
    ("up", 'A'),
    ("down", 'B'),
    ("right", 'C'),
    ("left", 'D'),
    ("home", 'H'),
    ("end", 'F'),
];

const TAB: &str = "tab";
const BACKSPACE: &str = "backspace";

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

// SPEC: https://github.com/xtermjs/xterm.js/blob/master/src/common/input/Keyboard.ts Ctrl+Shift sends only ^@ and ^_
const SHIFTED_CONTROL_KEYS: [&str; 3] = ["@", "_", "2"];

fn control_sequence(input: &KeyInput) -> Option<Vec<u8>> {
    let modifiers = input.modifiers;
    let shifted = modifiers.contains(Modifiers::SHIFT) && !modifiers.contains(Modifiers::ALT);
    if shifted && !SHIFTED_CONTROL_KEYS.contains(&input.key.as_str()) {
        return None;
    }
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

fn named_sequence(
    key: &str,
    modifiers: Modifiers,
    cursor_keys: CursorKeys,
) -> Option<Vec<u8>> {
    let parameter = modifiers.xterm_parameter();
    if let Some((_, code)) = CURSOR_KEYS.iter().find(|(name, _)| *name == key) {
        return Some(cursor_key(*code, parameter, cursor_keys).into_bytes());
    }
    let tilde_keys = TILDE_KEYS.iter().chain(FUNCTION_TILDE_CODES.iter());
    if let Some((_, code)) = tilde_keys.into_iter().find(|(name, _)| *name == key) {
        return Some(tilde_key(code, parameter).into_bytes());
    }
    if let Some((_, final_byte)) = SS3_FUNCTION_KEYS.iter().find(|(name, _)| *name == key) {
        return Some(ss3_key(*final_byte, parameter).into_bytes());
    }
    simple_key(key, modifiers)
}

fn cursor_key(
    final_byte: char,
    parameter: u8,
    cursor_keys: CursorKeys,
) -> String {
    if parameter > 1 {
        return format!("{ESC_TEXT}[1;{parameter}{final_byte}");
    }
    match cursor_keys {
        CursorKeys::Application => format!("{ESC_TEXT}O{final_byte}"),
        CursorKeys::Normal => format!("{ESC_TEXT}[{final_byte}"),
    }
}

fn tilde_key(
    code: &str,
    parameter: u8,
) -> String {
    if parameter > 1 {
        return format!("{ESC_TEXT}[{code};{parameter}~");
    }
    format!("{ESC_TEXT}[{code}~")
}

fn ss3_key(
    final_byte: char,
    parameter: u8,
) -> String {
    if parameter > 1 {
        return format!("{ESC_TEXT}[1;{parameter}{final_byte}");
    }
    format!("{ESC_TEXT}O{final_byte}")
}

fn simple_key(
    key: &str,
    modifiers: Modifiers,
) -> Option<Vec<u8>> {
    let alt = modifiers.contains(Modifiers::ALT);
    let sequence: &[u8] = match key {
        TAB if modifiers.contains(Modifiers::SHIFT) => b"\x1b[Z",
        TAB if alt => b"\x1b\t",
        TAB => b"\t",
        "escape" => b"\x1b",
        BACKSPACE if modifiers.contains(Modifiers::CONTROL) => b"\x08",
        BACKSPACE if alt => b"\x1b\x7f",
        BACKSPACE => b"\x7f",
        _ => return None,
    };
    Some(sequence.to_vec())
}

#[cfg(test)]
mod tests {
    use super::*;

    const NO: Modifiers = Modifiers::NONE;
    const SHIFT: Modifiers = Modifiers::SHIFT;
    const ALT: Modifiers = Modifiers::ALT;
    const CTRL: Modifiers = Modifiers::CONTROL;
    const CTRL_ALT: Modifiers = CTRL.with(ALT);
    const CTRL_SHIFT: Modifiers = CTRL.with(SHIFT);
    const NORMAL: CursorKeys = CursorKeys::Normal;
    const APP: CursorKeys = CursorKeys::Application;

    type Key = (&'static str, Option<&'static str>, Modifiers, CursorKeys);

    const CASES: &[(Key, Option<&str>)] = &[
        (("a", Some("a"), NO, NORMAL), Some("a")),
        (("a", Some("A"), SHIFT, NORMAL), Some("A")),
        (("c", Some("c"), CTRL, NORMAL), Some("\x03")),
        (("space", Some(" "), CTRL, NORMAL), Some("\0")),
        (("space", Some(" "), NO, NORMAL), Some(" ")),
        (("b", Some("b"), ALT, NORMAL), Some("\x1bb")),
        (("d", Some("d"), CTRL_ALT, NORMAL), Some("\x1b\x04")),
        (("]", Some("]"), CTRL, NORMAL), Some("\x1d")),
        (("é", Some("é"), NO, NORMAL), Some("é")),
        (("c", Some("c"), Modifiers::PLATFORM, NORMAL), None),
        (("f13", None, NO, NORMAL), None),
        (("n", Some("N"), CTRL_SHIFT, NORMAL), None),
        (("2", Some("@"), CTRL_SHIFT, NORMAL), Some("\0")),
        (("_", Some("_"), CTRL_SHIFT, NORMAL), Some("\x1f")),
        (("enter", None, NO, NORMAL), Some("\r")),
        (("enter", None, SHIFT, NORMAL), Some("\x1b[13;2u")),
        (("enter", None, ALT, NORMAL), Some("\x1b[13;3u")),
        (("up", None, NO, NORMAL), Some("\x1b[A")),
        (("up", None, NO, APP), Some("\x1bOA")),
        (("left", None, CTRL, APP), Some("\x1b[1;5D")),
        (("home", None, NO, APP), Some("\x1bOH")),
        (("pageup", None, NO, NORMAL), Some("\x1b[5~")),
        (("delete", None, SHIFT, NORMAL), Some("\x1b[3;2~")),
        (("f1", None, NO, NORMAL), Some("\x1bOP")),
        (("f4", None, ALT, NORMAL), Some("\x1b[1;3S")),
        (("f5", None, NO, NORMAL), Some("\x1b[15~")),
        (("f12", None, CTRL, NORMAL), Some("\x1b[24;5~")),
        (("tab", None, SHIFT, NORMAL), Some("\x1b[Z")),
        (("backspace", None, NO, NORMAL), Some("\x7f")),
        (("backspace", None, ALT, NORMAL), Some("\x1b\x7f")),
        (("backspace", None, CTRL, NORMAL), Some("\x08")),
        (("escape", None, NO, NORMAL), Some("\x1b")),
    ];

    fn press(&(key, text, modifiers, _): &Key) -> KeyInput {
        KeyInput {
            key: key.into(),
            text: text.map(str::to_owned),
            modifiers,
        }
    }

    #[test]
    fn keys_encode_by_modifier_and_cursor_mode() {
        for (key, expected) in CASES {
            let bytes = encode_key(&press(key), key.3);
            let wanted = expected.map(|text| text.as_bytes().to_vec());
            assert_eq!(bytes, wanted, "{key:?}");
        }
    }
}
