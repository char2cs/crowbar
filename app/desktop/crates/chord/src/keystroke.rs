use crate::dom_key::dom_key;
use crate::modifier::Modifier;
use crate::modifiers::Modifiers;

/// One key press as the shell's windowing layer reports it, free of any toolkit type.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Keystroke {
    /// The modifier keys held down with the key.
    pub modifiers: Modifiers,
    /// The key as the toolkit names it, for example `t`, `left` or `enter`.
    pub key: String,
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
            (stroke("mod", "shift"), None),
        ];
        for (input, expected) in cases {
            assert_eq!(input.chord().as_deref(), expected, "{input:?}");
        }
    }
}
