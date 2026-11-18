use protocol::{CommandId, KeyBinding, KeyChord};

use crate::keystroke::Keystroke;
use crate::modifier::Modifier;

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

/// The slot a `mod+shift+<digit>` key press selects; the chord is fixed, never rebound.
#[must_use]
pub fn project_slot_of_stroke(stroke: &Keystroke) -> Option<u8> {
    let held = |modifier| stroke.modifiers.contains(modifier);
    let is_chord = held(Modifier::Primary) && held(Modifier::Shift) && !held(Modifier::Alt);
    if !is_chord {
        return None;
    }
    let slot: u8 = stroke.key.parse().ok()?;
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

    fn stroke(
        modifiers: &str,
        key: &str,
    ) -> Keystroke {
        let held = [
            ('m', Modifier::Primary),
            ('s', Modifier::Shift),
            ('a', Modifier::Alt),
        ];
        Keystroke {
            modifiers: held
                .into_iter()
                .filter(|(letter, _)| modifiers.contains(*letter))
                .map(|(_, modifier)| modifier)
                .collect(),
            key: key.to_owned(),
        }
    }

    #[test]
    fn only_primary_shift_and_a_nonzero_digit_select_a_slot() {
        let cases = [
            (stroke("ms", "1"), Some(1)),
            (stroke("ms", "9"), Some(9)),
            (stroke("ms", "0"), None),
            (stroke("m", "1"), None),
            (stroke("msa", "1"), None),
            (stroke("s", "1"), None),
            (stroke("ms", "a"), None),
        ];
        for (press, expected) in cases {
            assert_eq!(project_slot_of_stroke(&press), expected, "{press:?}");
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
