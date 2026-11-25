//! Where the keys the shell hears sit on the keyboard, read from the X server's own keyboard
//! mapping, so a shortcut on a shifted digit still names its digit as develop's `event.code`.

use chord::{Modifier, Modifiers, PhysicalKey};
use gpui::{App, Global};

const ALL: [Modifier; 3] = [Modifier::Primary, Modifier::Shift, Modifier::Alt];

struct KeyPlaces(platform::KeyMap);

impl Global for KeyPlaces {}

pub(crate) fn install(cx: &mut App) {
    match platform::KeyMap::connect() {
        Ok(map) => cx.set_global(KeyPlaces(map)),
        Err(error) => tracing::warn!(%error, "the keyboard mapping is out of reach"),
    }
}

pub(crate) fn of(
    stroke: &gpui::Keystroke,
    modifiers: Modifiers,
    cx: &App,
) -> Option<PhysicalKey> {
    let KeyPlaces(map) = cx.try_global::<KeyPlaces>()?;
    let typed = single_char(&stroke.key)?;
    let place = map
        .us_key_of(typed)
        .inspect_err(|error| tracing::warn!(%error, "the keyboard mapping could not be read"))
        .ok()??;
    let shift = place.shifted.then_some(Modifier::Shift);
    let held = ALL
        .into_iter()
        .filter(|modifier| modifiers.contains(*modifier));
    Some(PhysicalKey {
        code: place.key.to_string(),
        modifiers: held.chain(shift).collect(),
    })
}

fn single_char(key: &str) -> Option<char> {
    let mut chars = key.chars();
    let first = chars.next()?;
    chars.next().is_none().then_some(first)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_one_character_key_has_a_place_to_look_up() {
        let cases = [
            ("2", Some('2')),
            ("@", Some('@')),
            ("enter", None),
            ("", None),
        ];
        for (key, expected) in cases {
            assert_eq!(single_char(key), expected, "{key:?}");
        }
    }
}
