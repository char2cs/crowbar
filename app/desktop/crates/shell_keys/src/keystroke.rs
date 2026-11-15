use crate::chord_text::dom_key;

/// One key press as the shell's windowing layer reports it, free of any toolkit type.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Keystroke {
    /// The platform primary modifier is down: Command on macOS, Control elsewhere.
    pub primary: bool,
    /// Shift is down.
    pub shift: bool,
    /// Alt or Option is down.
    pub alt: bool,
    /// The key as the toolkit names it, for example `t`, `left` or `enter`.
    pub key: String,
}

// SPEC: three modifiers and the key.
const MAX_PARTS: usize = 4;

impl Keystroke {
    /// The canonical chord text (`mod+shift+t`), the notation of the keybindings settings and
    /// of the pane script; `None` for a modifier pressed alone or an empty key.
    #[must_use]
    pub fn chord(&self) -> Option<String> {
        let key = dom_key(&self.key)?;
        let mut parts = Vec::with_capacity(MAX_PARTS);
        if self.primary {
            parts.push("mod");
        }
        if self.shift {
            parts.push("shift");
        }
        if self.alt {
            parts.push("alt");
        }
        parts.push(&key);
        Some(parts.join("+"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // SPEC: `modifiers` names the held ones, any of "mod", "shift" and "alt".
    fn stroke(
        modifiers: &str,
        key: &str,
    ) -> Keystroke {
        Keystroke {
            primary: modifiers.contains("mod"),
            shift: modifiers.contains("shift"),
            alt: modifiers.contains("alt"),
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
