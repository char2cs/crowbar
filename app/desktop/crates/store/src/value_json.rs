//! Setting values as the settings file stores them.

use settings_ui::Value;

pub(crate) fn to_json(value: &Value) -> serde_json::Value {
    match value {
        Value::Bool(flag) => serde_json::Value::Bool(*flag),
        Value::Text(text) => serde_json::Value::String(text.clone()),
        Value::Number(number) => serde_json::Number::from_f64(*number)
            .map_or(serde_json::Value::Null, serde_json::Value::Number),
        Value::List(items) => serde_json::json!(items),
        Value::Map(pairs) => serde_json::json!(pairs),
        _ => serde_json::Value::Null,
    }
}

pub(crate) fn from_json(json: &serde_json::Value) -> Option<Value> {
    match json {
        serde_json::Value::Bool(flag) => Some(Value::Bool(*flag)),
        serde_json::Value::String(text) => Some(Value::Text(text.clone())),
        serde_json::Value::Number(number) => number.as_f64().map(Value::Number),
        serde_json::Value::Array(items) => items
            .iter()
            .map(|item| item.as_str().map(str::to_owned))
            .collect::<Option<_>>()
            .map(Value::List),
        serde_json::Value::Object(pairs) => pairs
            .iter()
            .map(|(k, v)| v.as_str().map(|v| (k.clone(), v.to_owned())))
            .collect::<Option<_>>()
            .map(Value::Map),
        serde_json::Value::Null => None,
    }
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;

    use super::*;

    #[test]
    fn every_value_shape_survives_the_file() {
        let map = BTreeMap::from([("a".to_owned(), "b".to_owned())]);
        for value in [
            Value::Bool(true),
            Value::Text("x".into()),
            Value::Number(2.5),
            Value::List(vec!["a".into(), "b".into()]),
            Value::Map(map),
        ] {
            assert_eq!(from_json(&to_json(&value)), Some(value));
        }
        assert_eq!(from_json(&serde_json::Value::Null), None);
        assert_eq!(from_json(&serde_json::json!([1])), None);
    }
}
