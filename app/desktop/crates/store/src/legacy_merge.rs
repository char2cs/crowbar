//! Turning the old `crowbar:settings:*` entries into shell settings.

use std::collections::BTreeMap;

use settings::Value;

use crate::legacy_store::Entries;
use crate::value_json::from_json;

const PREFIX: &str = "crowbar:settings:";
const EXTRA_KEYS: [&str; 2] = ["theme", "themeMode"];

fn owned(key: &str) -> bool {
    protocol::PANE_SETTING_KEYS.contains(&key) || EXTRA_KEYS.contains(&key)
}

pub(crate) fn merge(
    current: &mut BTreeMap<String, Value>,
    entries: &Entries,
) -> usize {
    let adopted: Vec<(String, Value)> = entries
        .iter()
        .filter_map(|(name, text)| adoptable(current, name, text))
        .collect();
    let added = adopted.len();
    current.extend(adopted);
    added
}

fn adoptable(
    current: &BTreeMap<String, Value>,
    name: &str,
    text: &str,
) -> Option<(String, Value)> {
    let key = name.strip_prefix(PREFIX).filter(|key| owned(key))?;
    if current.contains_key(key) {
        return None;
    }
    let json = serde_json::from_str(text).ok()?;
    Some((key.to_owned(), from_json(&json)?))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_owned_unset_settings_are_taken() {
        let entries = BTreeMap::from([
            (
                "crowbar:settings:themeMode".to_owned(),
                "\"dark\"".to_owned(),
            ),
            ("crowbar:settings:tabSize".to_owned(), "4".to_owned()),
            ("crowbar:settings:fontSize".to_owned(), "15".to_owned()),
            ("crowbar:settings:unknownKey".to_owned(), "1".to_owned()),
            (
                "crowbar:settings:wordWrap".to_owned(),
                "not json".to_owned(),
            ),
            ("other".to_owned(), "1".to_owned()),
        ]);
        let mut current = BTreeMap::from([("fontSize".to_owned(), Value::Number(12.0))]);
        assert_eq!(merge(&mut current, &entries), 2);
        assert_eq!(
            current,
            BTreeMap::from([
                ("fontSize".to_owned(), Value::Number(12.0)),
                ("tabSize".to_owned(), Value::Number(4.0)),
                ("themeMode".to_owned(), Value::Text("dark".into())),
            ])
        );
    }
}
