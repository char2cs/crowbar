//! Translates GPUI key presses into the capture rule's vocabulary.

use gpui::Keystroke;

use crate::modifier::Modifier;
use crate::pressed::Pressed;

pub(crate) fn pressed(keystroke: &Keystroke) -> Pressed {
    let flags = [
        (keystroke.modifiers.control, Modifier::Control),
        (keystroke.modifiers.alt, Modifier::Alt),
        (keystroke.modifiers.shift, Modifier::Shift),
        (keystroke.modifiers.platform, Modifier::Platform),
    ];
    let held: Vec<Modifier> = flags
        .iter()
        .filter(|(on, _)| *on)
        .map(|&(_, modifier)| modifier)
        .collect();
    Pressed::new(dom_name(&keystroke.key), &held)
}

fn dom_name(key: &str) -> &str {
    match key {
        "left" => "arrowleft",
        "right" => "arrowright",
        "up" => "arrowup",
        "down" => "arrowdown",
        other => other,
    }
}

#[cfg(test)]
#[path = "keys_tests.rs"]
mod tests;
