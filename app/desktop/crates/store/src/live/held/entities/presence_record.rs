//! How many connections hold a chat, as the projects scope carries it.

use serde::Deserialize;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(default)]
pub(crate) struct PresenceRecord {
    pub(crate) viewers: u32,
}
