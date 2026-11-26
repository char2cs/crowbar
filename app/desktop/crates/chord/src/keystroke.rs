mod modifier;
mod modifiers;
mod physical_key;

pub use modifier::Modifier;
pub use modifiers::Modifiers;
pub use physical_key::PhysicalKey;

/// One key press as the shell's windowing layer reports it, free of any toolkit type.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Keystroke {
    /// The modifier keys held down with the key.
    pub modifiers: Modifiers,
    /// The key as the toolkit names it, for example `t`, `left` or `enter`.
    pub key: String,
    /// Where the key sits on the keyboard, when the windowing layer reports it.
    pub physical: Option<PhysicalKey>,
}

const CHORD_NAMES: [(Modifier, &str); 3] = [
    (Modifier::Primary, "mod"),
    (Modifier::Shift, "shift"),
    (Modifier::Alt, "alt"),
];

impl Keystroke {
    /// The canonical chord text (`mod+shift+t`), the notation of the keybindings settings and
    /// of the pane script; `None` for a modifier pressed alone or an empty key.
    #[must_use]
    pub fn chord(&self) -> Option<String> {
        let key = dom_key(&self.key)?;
        let held = CHORD_NAMES
            .iter()
            .filter(|(modifier, _)| self.modifiers.contains(*modifier))
            .map(|&(_, name)| name);
        let parts: Vec<&str> = held.chain(std::iter::once(key.as_str())).collect();
        Some(parts.join("+"))
    }
}

const MODIFIER_KEYS: &[&str] = &[
    "shift", "control", "ctrl", "alt", "option", "meta", "command", "cmd", "platform", "function",
    "fn", "super",
];

const RENAMED: &[(&str, &str)] = &[
    ("left", "arrowleft"),
    ("right", "arrowright"),
    ("up", "arrowup"),
    ("down", "arrowdown"),
    ("space", " "),
];

fn dom_key(key: &str) -> Option<String> {
    let lowered = key.to_lowercase();
    if lowered.is_empty() || MODIFIER_KEYS.contains(&lowered.as_str()) {
        return None;
    }
    let renamed = RENAMED
        .iter()
        .find(|(from, _)| *from == lowered)
        .map_or(lowered.as_str(), |(_, to)| *to);
    Some(renamed.to_owned())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn stroke(
        modifiers: &str,
        key: &str,
    ) -> Keystroke {
        let held = [
            ("mod", Modifier::Primary),
            ("shift", Modifier::Shift),
            ("alt", Modifier::Alt),
        ];
        Keystroke {
            modifiers: held
                .into_iter()
                .filter(|(name, _)| modifiers.contains(name))
                .map(|(_, modifier)| modifier)
                .collect(),
            key: key.to_owned(),
            physical: None,
        }
    }

    #[test]
    fn modifiers_come_in_the_canonical_order() {
        let cases = [
            (stroke("mod", "t"), Some("mod+t")),
            (stroke("mod shift", "T"), Some("mod+shift+t")),
            (
                stroke("mod shift alt", "left"),
                Some("mod+shift+alt+arrowleft"),
            ),
            (stroke("alt", "n"), Some("alt+n")),
            (stroke("", "escape"), Some("escape")),
            (stroke("", "space"), Some(" ")),
            (stroke("", "`"), Some("`")),
            (stroke("mod", "shift"), None),
            (stroke("", "control"), None),
            (stroke("", ""), None),
        ];
        for (input, expected) in cases {
            assert_eq!(input.chord().as_deref(), expected, "{input:?}");
        }
    }
}
