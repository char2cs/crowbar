//! The shell's own settings, kept in one JSON file of the state directory.

mod legacy_import;

pub use legacy_import::LegacyImport;

use std::collections::BTreeMap;
use std::path::PathBuf;
use std::sync::Arc;

use settings::Value;

use crate::error::{Error, Result};
use crate::live::{Files, read_text};

#[derive(Debug, Clone)]
pub(crate) struct JsonFile {
    path: PathBuf,
    files: Arc<dyn Files>,
}

impl JsonFile {
    pub(crate) fn new(
        path: PathBuf,
        files: Arc<dyn Files>,
    ) -> Self {
        Self { path, files }
    }

    pub(crate) fn load(&self) -> Result<BTreeMap<String, Value>> {
        let Some(text) = self.read()? else {
            return Ok(BTreeMap::new());
        };
        let stored: BTreeMap<String, serde_json::Value> =
            serde_json::from_str(&text).map_err(|source| self.json_error(source))?;
        Ok(stored
            .iter()
            .filter_map(|(key, json)| from_json(json).map(|value| (key.clone(), value)))
            .collect())
    }

    fn read(&self) -> Result<Option<String>> {
        read_text(self.files.as_ref(), &self.path).map_err(|source| self.file_error(source))
    }

    pub(crate) fn save(
        &self,
        values: &BTreeMap<String, Value>,
    ) -> Result<()> {
        let stored: BTreeMap<&String, serde_json::Value> = values
            .iter()
            .map(|(key, value)| (key, to_json(value)))
            .collect();
        let text =
            serde_json::to_string_pretty(&stored).map_err(|source| self.json_error(source))?;
        self.files
            .write(&self.path, text.as_bytes())
            .map_err(|source| self.file_error(source))
    }

    fn file_error(
        &self,
        source: std::io::Error,
    ) -> Error {
        Error::File {
            path: self.path.clone(),
            source,
        }
    }

    fn json_error(
        &self,
        source: serde_json::Error,
    ) -> Error {
        Error::Json {
            path: self.path.clone(),
            source,
        }
    }
}

fn to_json(value: &Value) -> serde_json::Value {
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

fn from_json(json: &serde_json::Value) -> Option<Value> {
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
    use super::*;
    use crate::test_support::FakeFiles;

    const PATH: &str = "/state/settings.json";

    fn file_on(files: &Arc<FakeFiles>) -> JsonFile {
        JsonFile::new(PATH.into(), Arc::<FakeFiles>::clone(files))
    }

    #[test]
    fn values_saved_are_loaded_and_a_missing_file_is_empty() {
        let file = file_on(&Arc::default());
        assert!(file.load().unwrap().is_empty());
        let pair = BTreeMap::from([("a".to_owned(), "b".to_owned())]);
        let values = BTreeMap::from([
            ("theme".to_owned(), Value::Text("dark".into())),
            ("tabSize".to_owned(), Value::Number(4.5)),
            ("wrap".to_owned(), Value::Bool(true)),
            ("fonts".to_owned(), Value::List(vec!["a".into(), "b".into()])),
            ("env".to_owned(), Value::Map(pair)),
        ]);
        file.save(&values).unwrap();
        assert_eq!(file.load().unwrap(), values);
    }

    #[test]
    fn a_file_that_is_not_json_is_an_error_and_values_it_cannot_hold_are_skipped() {
        let files = Arc::new(FakeFiles::default());
        files.put(PATH, b"{");
        assert!(matches!(file_on(&files).load(), Err(Error::Json { .. })));
        files.put(PATH, br#"{"a":null,"b":[1],"c":"x"}"#);
        let loaded = file_on(&files).load().unwrap();
        let kept = BTreeMap::from([("c".to_owned(), Value::Text("x".into()))]);
        assert_eq!(loaded, kept, "a null and a list of numbers are skipped");
    }
}
