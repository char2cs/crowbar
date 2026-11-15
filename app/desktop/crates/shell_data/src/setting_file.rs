//! The shell's own settings, kept in one JSON file of the state directory.

use std::collections::BTreeMap;
use std::path::PathBuf;

use settings_ui::Value;

use crate::error::{Error, Result};
use crate::value_json::{from_json, to_json};

#[derive(Debug, Clone)]
pub(crate) struct SettingFile {
    path: PathBuf,
}

impl SettingFile {
    pub(crate) fn new(path: PathBuf) -> Self {
        Self { path }
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
        match std::fs::read_to_string(&self.path) {
            Ok(text) => Ok(Some(text)),
            Err(source) if source.kind() == std::io::ErrorKind::NotFound => Ok(None),
            Err(source) => Err(self.file_error(source)),
        }
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
        self.ensure_directory()?;
        std::fs::write(&self.path, text).map_err(|source| self.file_error(source))
    }

    fn ensure_directory(&self) -> Result<()> {
        let Some(parent) = self.path.parent() else {
            return Ok(());
        };
        std::fs::create_dir_all(parent).map_err(|source| Error::File {
            path: parent.to_owned(),
            source,
        })
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn values_saved_are_loaded_and_a_missing_file_is_empty() {
        let dir = tempfile::tempdir().unwrap();
        let file = SettingFile::new(dir.path().join("nested/settings.json"));
        assert!(file.load().unwrap().is_empty());
        let values = BTreeMap::from([
            ("theme".to_owned(), Value::Text("dark".into())),
            ("tabSize".to_owned(), Value::Number(4.0)),
        ]);
        file.save(&values).unwrap();
        assert_eq!(file.load().unwrap(), values);
    }

    #[test]
    fn a_file_that_is_not_json_is_an_error_not_an_empty_set() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("settings.json");
        std::fs::write(&path, "{").unwrap();
        assert!(matches!(
            SettingFile::new(path).load(),
            Err(Error::Json { .. })
        ));
    }
}
