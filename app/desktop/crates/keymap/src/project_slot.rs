use protocol::{CommandId, KeyBinding, KeyChord};

use chord::Keystroke;
use chord::Modifier;

const SLOTS: std::ops::RangeInclusive<u8> = 1..=9;
const COMMAND_PREFIX: &str = "navigation.selectProject.";

fn command_id(slot: u8) -> String {
    format!("{COMMAND_PREFIX}{slot}")
}

/// The projects slot a command id names (`navigation.selectProject.3` is slot 3).
#[must_use]
pub fn project_slot_of_command(id: &str) -> Option<u8> {
    let slot: u8 = id.strip_prefix(COMMAND_PREFIX)?.parse().ok()?;
    SLOTS.contains(&slot).then_some(slot)
}

/// The slot a `mod+shift+<digit>` key press selects, by the digit key's position when the
/// windowing layer reports it, whatever shift and the layout make it type; never rebound.
#[must_use]
pub fn project_slot_of_stroke(stroke: &Keystroke) -> Option<u8> {
    let (modifiers, key) = stroke
        .physical
        .as_ref()
        .map_or((stroke.modifiers, &stroke.key), |at| {
            (at.modifiers, &at.code)
        });
    let held = |modifier| modifiers.contains(modifier);
    let is_chord = held(Modifier::Primary) && held(Modifier::Shift) && !held(Modifier::Alt);
    if !is_chord {
        return None;
    }
    let slot: u8 = key.parse().ok()?;
    SLOTS.contains(&slot).then_some(slot)
}

#[must_use]
pub(crate) fn project_bindings() -> Vec<KeyBinding> {
    SLOTS
        .filter_map(|slot| {
            let keys = KeyChord::new(format!("mod+shift+{slot}")).ok()?;
            let id = CommandId::new(command_id(slot)).ok()?;
            Some(KeyBinding::new(keys, id))
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::stroke;
    use chord::PhysicalKey;

    /// `typed`, at the key position and with the modifiers of `physical`.
    fn at(
        typed: &str,
        physical: &str,
    ) -> Keystroke {
        let place = stroke(physical);
        let physical = PhysicalKey {
            code: place.key,
            modifiers: place.modifiers,
        };
        Keystroke {
            physical: Some(physical),
            ..stroke(typed)
        }
    }

    #[test]
    fn primary_shift_and_the_digit_key_select_its_slot_whatever_the_layout_types() {
        let cases = [
            ("us shift+2 types @", at("mod+@", "mod+shift+2"), Some(2)),
            (
                "azerty shift+2 types 2",
                at("mod+2", "mod+shift+2"),
                Some(2),
            ),
            ("azerty 2 alone types é", at("mod+é", "mod+2"), None),
            ("us shift+a", at("mod+shift+a", "mod+shift+a"), None),
            ("first", stroke("mod+shift+1"), Some(1)),
            ("last", stroke("mod+shift+9"), Some(9)),
            ("zero", stroke("mod+shift+0"), None),
            ("no shift", stroke("mod+1"), None),
            ("alt too", stroke("mod+shift+alt+1"), None),
            ("no primary", stroke("shift+1"), None),
            ("a letter", stroke("mod+shift+a"), None),
        ];
        for (name, press, expected) in cases {
            assert_eq!(project_slot_of_stroke(&press), expected, "{name}");
        }
    }

    #[test]
    fn a_forwarded_binding_names_the_slot_it_selects() {
        let bindings = project_bindings();
        assert_eq!(bindings.len(), 9);
        for binding in &bindings {
            let slot = project_slot_of_command(binding.command().as_str());
            let expected = format!("mod+shift+{}", slot.unwrap());
            assert_eq!(binding.keys().as_str(), expected);
        }
        assert_eq!(project_slot_of_command("navigation.selectProject.10"), None);
        assert_eq!(project_slot_of_command("tabs.new"), None);
    }
}
