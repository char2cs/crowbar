//! What the settings writer has still to do.

use std::collections::BTreeMap;

use settings::Value;

#[derive(Debug, Default)]
pub(crate) struct PendingSave {
    pub(crate) latest: Option<BTreeMap<String, Value>>,
    pub(crate) writing: bool,
    pub(crate) closed: bool,
}
