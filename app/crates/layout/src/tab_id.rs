use serde::{Deserialize, Serialize};

/// Stable identifier of a tab, chosen by the caller and persisted with the layout.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
#[serde(transparent)]
pub struct TabId(pub u64);
