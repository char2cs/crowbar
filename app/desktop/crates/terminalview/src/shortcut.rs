//! The terminal's own keyboard shortcuts.

use terminal::{KeyInput, Modifiers};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Shortcut {
    Copy,
    Paste,
    SelectAll,
    Find,
    ZoomIn,
    ZoomOut,
    ZoomReset,
    ClearScrollback,
    PageUp,
    PageDown,
}

const CLIPBOARD_KEYS: [(&str, Shortcut); 2] = [("c", Shortcut::Copy), ("v", Shortcut::Paste)];
const PLATFORM_KEYS: [(&str, Shortcut); 2] =
    [("a", Shortcut::SelectAll), ("k", Shortcut::ClearScrollback)];
const PRIMARY_KEYS: [(&str, Shortcut); 5] = [
    ("f", Shortcut::Find),
    ("=", Shortcut::ZoomIn),
    ("+", Shortcut::ZoomIn),
    ("-", Shortcut::ZoomOut),
    ("0", Shortcut::ZoomReset),
];
const SHIFT_KEYS: [(&str, Shortcut); 2] = [
    ("pageup", Shortcut::PageUp),
    ("pagedown", Shortcut::PageDown),
];

fn lookup(
    table: &[(&str, Shortcut)],
    key: &str,
) -> Option<Shortcut> {
    table
        .iter()
        .find(|(name, _)| *name == key)
        .map(|(_, shortcut)| *shortcut)
}

pub(crate) fn shortcut_for(input: &KeyInput) -> Option<Shortcut> {
    let modifiers = input.modifiers;
    let platform = modifiers.contains(Modifiers::PLATFORM);
    let primary = platform || modifiers.contains(Modifiers::CONTROL);
    let chord = platform || modifiers.contains(Modifiers::CONTROL.with(Modifiers::SHIFT));
    let key = input.key.as_str();
    let tables = [
        (chord, &CLIPBOARD_KEYS[..]),
        (platform, &PLATFORM_KEYS[..]),
        (primary, &PRIMARY_KEYS[..]),
        (modifiers.contains(Modifiers::SHIFT), &SHIFT_KEYS[..]),
    ];
    tables
        .into_iter()
        .filter(|(active, _)| *active)
        .find_map(|(_, table)| lookup(table, key))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn press(
        key: &str,
        modifiers: Modifiers,
    ) -> KeyInput {
        KeyInput {
            key: key.into(),
            text: None,
            modifiers,
        }
    }

    #[test]
    fn shortcuts_follow_the_web_terminal() {
        let ctrl_shift = Modifiers::CONTROL.with(Modifiers::SHIFT);
        let cases = [
            (press("c", ctrl_shift), Some(Shortcut::Copy)),
            (press("c", Modifiers::CONTROL), None),
            (press("v", ctrl_shift), Some(Shortcut::Paste)),
            (press("v", Modifiers::PLATFORM), Some(Shortcut::Paste)),
            (press("f", Modifiers::CONTROL), Some(Shortcut::Find)),
            (
                press("k", Modifiers::PLATFORM),
                Some(Shortcut::ClearScrollback),
            ),
            (press("k", Modifiers::CONTROL), None),
            (press("=", Modifiers::CONTROL), Some(Shortcut::ZoomIn)),
            (press("-", Modifiers::PLATFORM), Some(Shortcut::ZoomOut)),
            (press("0", Modifiers::CONTROL), Some(Shortcut::ZoomReset)),
            (press("a", Modifiers::PLATFORM), Some(Shortcut::SelectAll)),
            (press("pageup", Modifiers::SHIFT), Some(Shortcut::PageUp)),
            (press("pageup", Modifiers::NONE), None),
            (press("l", Modifiers::CONTROL), None),
        ];
        for (input, expected) in cases {
            assert_eq!(shortcut_for(&input), expected, "{input:?}");
        }
    }
}
