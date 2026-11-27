use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum Lifecycle {
    Mount,
    Activate,
    Deactivate,
    Park,
    Destroy,
}
