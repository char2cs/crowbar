use chord::{Keystroke, Modifier};

/// The key press of `chord`, written `mod+shift+t`, with no physical position.
pub(crate) fn stroke(chord: &str) -> Keystroke {
    let mut parts: Vec<&str> = chord.split('+').collect();
    let key = parts.pop().unwrap_or_default().to_owned();
    let held = [
        ("mod", Modifier::Primary),
        ("shift", Modifier::Shift),
        ("alt", Modifier::Alt),
    ];
    Keystroke {
        modifiers: held
            .into_iter()
            .filter(|(name, _)| parts.contains(name))
            .map(|(_, modifier)| modifier)
            .collect(),
        key,
        physical: None,
    }
}
