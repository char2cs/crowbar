use protocol::{PANE_SETTING_KEYS, SettingValues};
use serde_json::{Number, Value as Json, json};
use settings_ui::{Key, Value};

pub(crate) fn follows(key: &Key) -> bool {
    PANE_SETTING_KEYS.contains(&key.as_str())
}

fn json_of(value: &Value) -> Option<Json> {
    match value {
        Value::Bool(flag) => Some(Json::Bool(*flag)),
        Value::Text(text) => Some(Json::String(text.clone())),
        Value::Number(number) => Number::from_f64(*number).map(Json::Number),
        Value::List(items) => Some(json!(items)),
        Value::Map(pairs) => Some(json!(pairs)),
        _ => None,
    }
}

pub(crate) fn values_of(stored: impl Fn(&Key) -> Option<Value>) -> protocol::Result<SettingValues> {
    PANE_SETTING_KEYS
        .iter()
        .try_fold(SettingValues::default(), |values, name| {
            match stored(&Key::new(*name)).as_ref().and_then(json_of) {
                Some(json) => values.with(*name, json),
                None => Ok(values),
            }
        })
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;

    use super::*;

    fn stored(key: &Key) -> Option<Value> {
        match key.as_str() {
            "wordWrap" => Some(Value::Bool(true)),
            "tabSize" => Some(Value::Number(4.0)),
            "browserSearchEngine" => Some(Value::from("custom")),
            "hiddenFilePatterns" => Some(Value::List(vec!["*.log".to_owned()])),
            "fontSize" => Some(Value::Number(f64::NAN)),
            "theme" => Some(Value::from("frosted")),
            "keybindingUserOverrides" => Some(Value::Map(BTreeMap::new())),
            _ => None,
        }
    }

    #[test]
    fn stored_values_the_panes_follow_become_json_and_the_rest_stay_home() {
        let values = values_of(stored).unwrap();
        assert_eq!(
            serde_json::to_value(&values).unwrap(),
            json!({
                "wordWrap": true,
                "tabSize": 4.0,
                "browserSearchEngine": "custom",
                "hiddenFilePatterns": ["*.log"],
            }),
            "followed values only; a non-finite number and shell-owned keys are left out"
        );
    }

    #[test]
    fn nothing_stored_means_an_empty_set() {
        assert!(values_of(|_| None).unwrap().is_empty(), "empty");
    }

    #[test]
    fn only_followed_keys_wake_the_push() {
        for (name, expected) in [
            ("wordWrap", true),
            ("browserZoom", true),
            ("theme", false),
            ("sidebarPosition", false),
        ] {
            assert_eq!(follows(&Key::new(name)), expected, "{name}");
        }
    }
}
