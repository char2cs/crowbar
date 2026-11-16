//! The WebKit `localStorage` database of the previous desktop app.

use std::collections::BTreeMap;
use std::path::PathBuf;

use rusqlite::{Connection, OpenFlags};

use crate::error::{Error, Result};
use crate::legacy_store::LegacyStore;

const QUERY: &str = "SELECT key, value FROM ItemTable";

#[derive(Debug, Clone)]
pub(crate) struct LegacySqlite {
    path: PathBuf,
}

impl LegacySqlite {
    pub(crate) fn new(path: PathBuf) -> Self {
        Self { path }
    }

    fn failed(
        &self,
        source: rusqlite::Error,
    ) -> Error {
        Error::Legacy {
            path: self.path.clone(),
            source,
        }
    }
}

// PLATFORM: WebKit stores a value as UTF-16LE text, or as Latin-1 when every character fits one byte.
fn decode(bytes: &[u8]) -> Option<String> {
    if let Some((&1, latin1)) = bytes.split_first() {
        return Some(latin1.iter().map(|byte| char::from(*byte)).collect());
    }
    let units: Vec<u16> = bytes
        .as_chunks::<2>()
        .0
        .iter()
        .map(|pair| u16::from_le_bytes(*pair))
        .collect();
    String::from_utf16(&units).ok()
}

impl LegacyStore for LegacySqlite {
    fn entries(&self) -> Result<BTreeMap<String, String>> {
        let flags = OpenFlags::SQLITE_OPEN_READ_ONLY;
        let db = Connection::open_with_flags(&self.path, flags).map_err(|e| self.failed(e))?;
        let mut statement = db.prepare(QUERY).map_err(|e| self.failed(e))?;
        let rows = statement
            .query_map([], |row| {
                Ok((row.get::<_, String>(0)?, row.get::<_, Vec<u8>>(1)?))
            })
            .map_err(|e| self.failed(e))?;
        rows.map(|row| row.map_err(|e| self.failed(e))).try_fold(
            BTreeMap::new(),
            |mut entries, row| {
                let (key, value) = row?;
                entries.extend(decode(&value).map(|text| (key, text)));
                Ok(entries)
            },
        )
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn utf16(text: &str) -> Vec<u8> {
        text.encode_utf16().flat_map(u16::to_le_bytes).collect()
    }

    #[test]
    fn values_decode_from_utf16_and_latin1() {
        let cases = [(utf16("\"dark\""), "\"dark\""), (vec![1, b'4'], "4")];
        for (bytes, text) in cases {
            assert_eq!(decode(&bytes).as_deref(), Some(text));
        }
    }

    #[test]
    fn a_database_of_the_old_app_reads_back_as_entries() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("tauri_localhost_0.localstorage");
        let db = Connection::open(&path).unwrap();
        db.execute("CREATE TABLE ItemTable (key TEXT, value BLOB)", [])
            .unwrap();
        let value = utf16("\"dark\"");
        db.execute(
            "INSERT INTO ItemTable VALUES ('crowbar:settings:themeMode', ?1)",
            [value],
        )
        .unwrap();
        drop(db);
        let entries = LegacySqlite::new(path).entries().unwrap();
        assert_eq!(
            entries
                .get("crowbar:settings:themeMode")
                .map(String::as_str),
            Some("\"dark\"")
        );
    }

    #[test]
    fn a_missing_database_is_an_error_with_its_path() {
        let missing = PathBuf::from("/nonexistent/old.localstorage");
        let error = LegacySqlite::new(missing.clone()).entries().unwrap_err();
        assert!(error.to_string().contains("/nonexistent/old.localstorage"));
    }
}
