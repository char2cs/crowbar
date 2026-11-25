//! Converts GPUI keystrokes to the terminal crate's key input.

use gpui::Keystroke;
use terminal::{KeyInput, Modifiers};

pub(crate) fn key_input(keystroke: &Keystroke) -> KeyInput {
    let modifiers = modifiers_of(keystroke.modifiers);
    let text = typed_text(keystroke, modifiers);
    KeyInput {
        key: keystroke.key.clone(),
        text,
        modifiers,
    }
}

fn typed_text(
    keystroke: &Keystroke,
    modifiers: Modifiers,
) -> Option<String> {
    if modifiers.contains(Modifiers::ALT) {
        return meta_text(keystroke, modifiers);
    }
    keystroke.key_char.clone()
}

fn meta_text(
    keystroke: &Keystroke,
    modifiers: Modifiers,
) -> Option<String> {
    let mut characters = keystroke.key.chars();
    let (Some(only), None) = (characters.next(), characters.next()) else {
        return keystroke.key_char.clone();
    };
    if modifiers.contains(Modifiers::SHIFT) {
        return Some(only.to_uppercase().collect());
    }
    Some(only.to_string())
}

fn modifiers_of(held: gpui::Modifiers) -> Modifiers {
    let flags = [
        (held.shift, Modifiers::SHIFT),
        (held.alt, Modifiers::ALT),
        (held.control, Modifiers::CONTROL),
        (held.platform, Modifiers::PLATFORM),
    ];
    flags
        .into_iter()
        .filter(|(down, _)| *down)
        .fold(Modifiers::NONE, |all, (_, modifier)| all.with(modifier))
}

pub(crate) fn terminal_modifiers(held: gpui::Modifiers) -> Modifiers {
    modifiers_of(held)
}

pub(crate) fn link_modifier_held(modifiers: gpui::Modifiers) -> bool {
    if cfg!(target_os = "macos") {
        return modifiers.platform;
    }
    modifiers.control
}

#[cfg(test)]
mod tests {
    use super::*;

    fn keystroke(source: &'static str) -> Keystroke {
        Keystroke::parse(source).unwrap_or_else(|_| Keystroke {
            modifiers: gpui::Modifiers::default(),
            key: source.to_owned(),
            key_char: None,
        })
    }

    #[test]
    fn the_link_modifier_is_the_platform_key_on_macos_and_control_elsewhere() {
        let held = |control: bool, platform: bool| gpui::Modifiers {
            control,
            platform,
            ..gpui::Modifiers::default()
        };
        let macos = cfg!(target_os = "macos");
        let cases = [
            ("control", held(true, false), !macos),
            ("platform", held(false, true), macos),
            ("none", held(false, false), false),
        ];
        for (name, modifiers, expected) in cases {
            assert_eq!(link_modifier_held(modifiers), expected, "{name}");
        }
    }

    #[test]
    fn modifiers_and_text_carry_over() {
        let plain = key_input(&keystroke("a"));
        assert_eq!(
            (plain.key.as_str(), plain.modifiers),
            ("a", Modifiers::NONE)
        );
        let control = key_input(&keystroke("ctrl-c"));
        assert!(control.modifiers.contains(Modifiers::CONTROL));
    }

    #[test]
    fn alt_types_the_key_not_the_composed_character() {
        let mut stroke = keystroke("alt-s");
        stroke.key_char = Some("ß".to_owned());
        assert_eq!(key_input(&stroke).text.as_deref(), Some("s"));
        let mut shifted = keystroke("alt-shift-s");
        shifted.key_char = Some("Í".to_owned());
        assert_eq!(key_input(&shifted).text.as_deref(), Some("S"));
    }
}
