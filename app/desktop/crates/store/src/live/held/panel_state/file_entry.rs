use serde::Deserialize;

use daemonapi::EntryKind;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(default)]
pub(crate) struct FileEntry {
    pub(crate) name: String,
    pub(crate) path: String,
    #[serde(rename = "type")]
    pub(crate) kind: String,
}

impl FileEntry {
    pub(crate) fn is_directory(&self) -> bool {
        self.kind == "directory"
    }

    pub(crate) fn entry_kind(&self) -> EntryKind {
        if self.is_directory() {
            return EntryKind::Folder;
        }
        EntryKind::File
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_listing_line_parses_and_tells_directories_apart() {
        let dir: FileEntry =
            serde_json::from_str(r#"{"name":"src","path":"src","type":"directory"}"#).unwrap();
        let file: FileEntry =
            serde_json::from_str(r#"{"name":"a","path":"a","type":"file"}"#).unwrap();
        assert!(dir.is_directory());
        assert!(!file.is_directory());
    }
}
