//! A directory entry as `GET /v0/system/browse` sends it.

use serde::Deserialize;

use crate::browse_entry::BrowseEntry;
use crate::entry_kind::EntryKind;
use crate::switch::Switch;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct BrowseWire {
    name: String,
    path: String,
    is_dir: Switch,
}

impl Default for BrowseWire {
    fn default() -> Self {
        Self {
            name: String::new(),
            path: String::new(),
            is_dir: Switch::Off,
        }
    }
}

impl BrowseWire {
    pub(crate) fn into_entry(self) -> BrowseEntry {
        let kind = match self.is_dir {
            Switch::On => EntryKind::Folder,
            Switch::Off => EntryKind::File,
        };
        BrowseEntry {
            name: self.name,
            path: self.path,
            kind,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(&str, EntryKind); 3] = [
        (
            r#"{"name":"src","path":"/a/src","isDir":true}"#,
            EntryKind::Folder,
        ),
        (
            r#"{"name":"a.txt","path":"/a/a.txt","isDir":false}"#,
            EntryKind::File,
        ),
        (r#"{"name":"x","path":"/a/x"}"#, EntryKind::File),
    ];

    #[test]
    fn an_entry_is_a_folder_only_when_the_daemon_says_so() {
        for (json, kind) in CASES {
            let entry = serde_json::from_str::<BrowseWire>(json)
                .unwrap()
                .into_entry();
            assert_eq!(entry.kind, kind, "{json}");
            assert!(entry.path.starts_with("/a/"));
        }
    }
}
