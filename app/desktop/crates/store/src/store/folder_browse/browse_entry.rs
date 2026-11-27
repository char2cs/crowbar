use daemonapi::EntryKind;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct BrowseEntry {
    pub name: String,
    pub path: String,
    pub kind: EntryKind,
}
