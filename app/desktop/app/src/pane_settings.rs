use std::collections::BTreeMap;

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

pub(crate) fn value_of(json: &Json) -> Option<Value> {
    match json {
        Json::Bool(flag) => Some(Value::Bool(*flag)),
        Json::String(text) => Some(Value::Text(text.clone())),
        Json::Number(number) => number.as_f64().map(Value::Number),
        Json::Array(items) => items
            .iter()
            .map(|item| item.as_str().map(str::to_owned))
            .collect::<Option<Vec<_>>>()
            .map(Value::List),
        Json::Object(pairs) => pairs
            .iter()
            .map(|(key, item)| item.as_str().map(|text| (key.clone(), text.to_owned())))
            .collect::<Option<BTreeMap<_, _>>>()
            .map(Value::Map),
        Json::Null => None,
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
    use super::*;

    struct Case {
        name: &'static str,
        stored: Option<Value>,
        json: Option<Json>,
    }

    fn stored_value(key: &Key) -> Option<Value> {
        match key.as_str() {
            "wordWrap" => Some(Value::Bool(true)),
            "tabSize" => Some(Value::Number(4.0)),
            "browserSearchEngine" => Some(Value::from("custom")),
            "hiddenFilePatterns" => Some(Value::List(vec!["*.log".to_owned()])),
            "fontSize" => Some(Value::Number(f64::NAN)),
            "theme" => Some(Value::from("frosted")),
            _ => None,
        }
    }

    #[test]
    fn stored_values_the_panes_follow_become_json_and_the_rest_stay_home() {
        let values = values_of(stored_value).unwrap();
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
        assert!(values_of(|_| None).unwrap().is_empty(), "nothing stored");
    }

    fn case(
        name: &'static str,
        stored: Option<Value>,
        json: Json,
    ) -> Case {
        Case {
            name,
            stored,
            json: Some(json),
        }
    }

    #[test]
    fn json_converts_to_the_shapes_the_catalog_stores() {
        let list = Value::List(vec!["a".to_owned()]);
        let pairs = Value::Map(BTreeMap::from([("k".to_owned(), "v".to_owned())]));
        let cases = vec![
            case("flag", Some(Value::Bool(true)), json!(true)),
            case("text", Some(Value::from("x")), json!("x")),
            case("number", Some(Value::Number(4.0)), json!(4)),
            case("list", Some(list), json!(["a"])),
            case("pairs", Some(pairs), json!({"k": "v"})),
            case("a list of numbers is no list of text", None, json!([1])),
            case("null", None, Json::Null),
        ];
        for case in cases {
            let json = case.json.unwrap();
            assert_eq!(value_of(&json), case.stored, "{}", case.name);
        }
    }

    #[test]
    fn only_followed_keys_wake_the_push() {
        for (name, expected) in [
            ("wordWrap", Some(())),
            ("browserZoom", Some(())),
            ("theme", None),
            ("sidebarPosition", None),
        ] {
            assert_eq!(follows(&Key::new(name)).then_some(()), expected, "{name}");
        }
    }
}
