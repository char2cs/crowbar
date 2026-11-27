use std::collections::BTreeMap;

use crate::shell::git::changed_file::ChangedFile;

#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct Changes {
    pub branch: String,
    pub parent: Option<String>,
    pub files: Vec<ChangedFile>,
    pub threads: BTreeMap<String, u32>,
    pub viewed: BTreeMap<String, String>,
    pub error: Option<String>,
}
