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

pub(crate) fn dom_key(key: &str) -> Option<String> {
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

    const CASES: &[(&str, Option<&str>)] = &[
        ("t", Some("t")),
        ("T", Some("t")),
        ("left", Some("arrowleft")),
        ("down", Some("arrowdown")),
        ("`", Some("`")),
        ("enter", Some("enter")),
        ("shift", None),
        ("control", None),
        ("", None),
    ];

    #[test]
    fn keys_use_the_page_names() {
        for (key, expected) in CASES {
            assert_eq!(dom_key(key).as_deref(), *expected, "{key:?}");
        }
    }
}
