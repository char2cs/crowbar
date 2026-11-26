//! One WebKit `IndexedDB` database: its `editor-drafts` records read, then removed.

use std::cmp::Ordering;
use std::path::Path;

use rusqlite::{Connection, OpenFlags, OptionalExtension as _};

use crate::error::{Error, Result};
use crate::store::legacy_drafts::script_value::ScriptValue;

const STORE: &str = "editor-drafts";
const KEY_COLLATION: &str = "IDBKEY";
const STRING_KEY: [u8; 2] = [0, 0x60];
const KEY_HEADER: usize = 6;

pub(crate) fn take_drafts(path: &Path) -> Result<Vec<editor::Draft>> {
    let failed = |source| Error::Legacy {
        path: path.to_path_buf(),
        source,
    };
    let mut db =
        Connection::open_with_flags(path, OpenFlags::SQLITE_OPEN_READ_WRITE).map_err(failed)?;
    db.create_collation(KEY_COLLATION, compare_keys)
        .map_err(failed)?;
    let store: Option<i64> = db
        .query_row(
            "SELECT id FROM ObjectStoreInfo WHERE name = ?1",
            [STORE],
            |row| row.get(0),
        )
        .optional()
        .map_err(failed)?;
    let Some(store) = store else {
        return Ok(Vec::new());
    };
    let drafts = read(&db, store).map_err(failed)?;
    if let Err(error) = remove(&mut db, store) {
        tracing::warn!(%error, path = %path.display(), "imported drafts stay in the old database");
    }
    Ok(drafts)
}

fn read(
    db: &Connection,
    store: i64,
) -> rusqlite::Result<Vec<editor::Draft>> {
    let mut statement = db.prepare("SELECT value FROM Records WHERE objectStoreID = ?1")?;
    let values = statement.query_map([store], |row| row.get::<_, Vec<u8>>(0))?;
    let values: Vec<Vec<u8>> = values.collect::<rusqlite::Result<_>>()?;
    Ok(values.iter().filter_map(|value| draft_of(value)).collect())
}

fn remove(
    db: &mut Connection,
    store: i64,
) -> rusqlite::Result<()> {
    let removal = db.transaction()?;
    removal.execute("DELETE FROM IndexRecords WHERE objectStoreID = ?1", [store])?;
    removal.execute("DELETE FROM Records WHERE objectStoreID = ?1", [store])?;
    removal.commit()
}

fn draft_of(value: &[u8]) -> Option<editor::Draft> {
    let mut strings = ScriptValue::strings_of(value)?;
    Some(editor::Draft {
        uri: strings.remove("uri")?,
        text: strings.remove("text")?,
        saved_text: strings.remove("savedText")?,
    })
}

// PLATFORM: WebKit orders a string key by its UTF-16 text; the column holds the key's encoding.
fn compare_keys(
    left: &str,
    right: &str,
) -> Ordering {
    match (key_text(left), key_text(right)) {
        (Some(left), Some(right)) => left.cmp(&right),
        _ => left.cmp(right),
    }
}

fn key_text(key: &str) -> Option<Vec<u16>> {
    let bytes = key.as_bytes();
    if bytes.get(..STRING_KEY.len())? != STRING_KEY {
        return None;
    }
    let units = bytes.get(KEY_HEADER..)?.as_chunks::<2>().0;
    Some(units.iter().map(|pair| u16::from_le_bytes(*pair)).collect())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{draft_bytes, latin1};

    fn key(text: &str) -> String {
        let units: Vec<u16> = text.encode_utf16().collect();
        let mut bytes = STRING_KEY.to_vec();
        bytes.extend(u32::try_from(units.len()).unwrap().to_le_bytes());
        bytes.extend(units.iter().flat_map(|unit| unit.to_le_bytes()));
        String::from_utf8(bytes).unwrap()
    }

    #[test]
    fn string_keys_order_by_their_text_not_their_length() {
        assert_eq!(compare_keys(&key("b"), &key("aa")), Ordering::Greater);
        assert_eq!(compare_keys(&key("a"), &key("a")), Ordering::Equal);
    }

    #[test]
    #[ignore = "real-infra: reads and edits a sqlite file laid out as WebKit's IndexedDB"]
    fn a_database_s_drafts_are_read_then_removed_and_other_stores_stay() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("IndexedDB.sqlite3");
        let db = Connection::open(&path).unwrap();
        db.create_collation(KEY_COLLATION, compare_keys).unwrap();
        db.execute_batch(
            "CREATE TABLE ObjectStoreInfo (id INTEGER PRIMARY KEY, name TEXT);
             CREATE TABLE Records (objectStoreID INTEGER, key TEXT COLLATE IDBKEY, value, recordID INTEGER PRIMARY KEY);
             CREATE UNIQUE INDEX RecordsIndex ON Records (objectStoreID, key);
             CREATE TABLE IndexRecords (indexID INTEGER, objectStoreID INTEGER, key TEXT COLLATE IDBKEY, value TEXT COLLATE IDBKEY, objectStoreRecordID INTEGER);
             INSERT INTO ObjectStoreInfo VALUES (91, 'editor-drafts'), (75, 'workspace-layout');",
        )
        .unwrap();
        let uri = "crowbar://editor/w/src%2Fa.ts";
        let value = draft_bytes(uri, &latin1("b"), &latin1("a"));
        for (store, text) in [(91, uri), (75, "window")] {
            db.execute(
                "INSERT INTO Records (objectStoreID, key, value) VALUES (?1, ?2, ?3)",
                rusqlite::params![store, key(text), value],
            )
            .unwrap();
        }
        drop(db);
        let drafts = take_drafts(&path).unwrap();
        assert_eq!(drafts.len(), 1);
        assert_eq!(
            (drafts[0].uri.as_str(), drafts[0].text.as_str()),
            (uri, "b")
        );
        assert!(take_drafts(&path).unwrap().is_empty(), "removed");
        let db = Connection::open(&path).unwrap();
        db.create_collation(KEY_COLLATION, compare_keys).unwrap();
        let left: i64 = db
            .query_row("SELECT count(*) FROM Records", [], |row| row.get(0))
            .unwrap();
        assert_eq!(left, 1, "the layout stays");
    }
}
