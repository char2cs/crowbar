//! Escape sequences of the non-printing keys.

use crate::cursor_keys::CursorKeys;
use crate::modifiers::Modifiers;

const ESC: &str = "\x1b";
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

pub(crate) fn named_sequence(
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
        return format!("{ESC}[1;{parameter}{final_byte}");
    }
    match cursor_keys {
        CursorKeys::Application => format!("{ESC}O{final_byte}"),
        CursorKeys::Normal => format!("{ESC}[{final_byte}"),
    }
}

fn tilde_key(
    code: &str,
    parameter: u8,
) -> String {
    if parameter > 1 {
        return format!("{ESC}[{code};{parameter}~");
    }
    format!("{ESC}[{code}~")
}

fn ss3_key(
    final_byte: char,
    parameter: u8,
) -> String {
    if parameter > 1 {
        return format!("{ESC}[1;{parameter}{final_byte}");
    }
    format!("{ESC}O{final_byte}")
}

fn simple_key(
    key: &str,
    modifiers: Modifiers,
) -> Option<Vec<u8>> {
    let alt = modifiers.contains(Modifiers::ALT);
    let sequence: &[u8] = match key {
        "tab" if modifiers.contains(Modifiers::SHIFT) => b"\x1b[Z",
        "tab" if alt => b"\x1b\t",
        "tab" => b"\t",
        "escape" => b"\x1b",
        "backspace" if modifiers.contains(Modifiers::CONTROL) => b"\x08",
        "backspace" if alt => b"\x1b\x7f",
        "backspace" => b"\x7f",
        _ => return None,
    };
    Some(sequence.to_vec())
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [Case; 16] = [
        ("up", Modifiers::NONE, CursorKeys::Normal, Some("\x1b[A")),
        (
            "up",
            Modifiers::NONE,
            CursorKeys::Application,
            Some("\x1bOA"),
        ),
        (
            "left",
            Modifiers::CONTROL,
            CursorKeys::Application,
            Some("\x1b[1;5D"),
        ),
        (
            "home",
            Modifiers::NONE,
            CursorKeys::Application,
            Some("\x1bOH"),
        ),
        (
            "pageup",
            Modifiers::NONE,
            CursorKeys::Normal,
            Some("\x1b[5~"),
        ),
        (
            "delete",
            Modifiers::SHIFT,
            CursorKeys::Normal,
            Some("\x1b[3;2~"),
        ),
        ("f1", Modifiers::NONE, CursorKeys::Normal, Some("\x1bOP")),
        ("f4", Modifiers::ALT, CursorKeys::Normal, Some("\x1b[1;3S")),
        ("f5", Modifiers::NONE, CursorKeys::Normal, Some("\x1b[15~")),
        (
            "f12",
            Modifiers::CONTROL,
            CursorKeys::Normal,
            Some("\x1b[24;5~"),
        ),
        ("tab", Modifiers::SHIFT, CursorKeys::Normal, Some("\x1b[Z")),
        (
            "backspace",
            Modifiers::NONE,
            CursorKeys::Normal,
            Some("\x7f"),
        ),
        (
            "backspace",
            Modifiers::ALT,
            CursorKeys::Normal,
            Some("\x1b\x7f"),
        ),
        (
            "backspace",
            Modifiers::CONTROL,
            CursorKeys::Normal,
            Some("\x08"),
        ),
        ("escape", Modifiers::NONE, CursorKeys::Normal, Some("\x1b")),
        ("a", Modifiers::NONE, CursorKeys::Normal, None),
    ];

    type Case = (&'static str, Modifiers, CursorKeys, Option<&'static str>);

    #[test]
    fn named_keys_encode_by_modifier_and_cursor_mode() {
        let cases = CASES;
        for (key, modifiers, cursor_keys, expected) in cases {
            let bytes = named_sequence(key, modifiers, cursor_keys)
                .map(|bytes| String::from_utf8_lossy(&bytes).into_owned());
            assert_eq!(bytes.as_deref(), expected, "{key} {modifiers:?}");
        }
    }
}
