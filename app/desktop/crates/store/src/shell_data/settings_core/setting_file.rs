//! The shell's own settings, kept in one JSON file of the state directory.

mod legacy_import;
mod stored_value;

pub use legacy_import::LegacyImport;

use std::collections::BTreeMap;
use std::path::PathBuf;
use std::sync::Arc;

use settings::Value;

use crate::error::{Error, Result};
use crate::live::{FileStore, read_text};
use crate::shell_data::settings_core::setting_file::stored_value::{from_json, to_json};

#[derive(Debug, Clone)]
pub(crate) struct SettingFile {
    path: PathBuf,
    files: Arc<dyn FileStore>,
}

impl SettingFile {
    pub(crate) fn new(
        path: PathBuf,
        files: Arc<dyn FileStore>,
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::FakeFiles;

    const PATH: &str = "/state/settings.json";

    fn file_on(files: &Arc<FakeFiles>) -> SettingFile {
        SettingFile::new(PATH.into(), Arc::<FakeFiles>::clone(files))
    }

    #[test]
    fn values_saved_are_loaded_and_a_missing_file_is_empty() {
        let file = file_on(&Arc::default());
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
        let files = Arc::new(FakeFiles::default());
        files.put(PATH, b"{");
        assert!(matches!(file_on(&files).load(), Err(Error::Json { .. })));
    }
}
