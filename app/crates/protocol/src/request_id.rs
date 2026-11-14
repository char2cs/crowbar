use serde::{Deserialize, Serialize};

/// Correlates a `request` or `subscribe` frame with its answer.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
#[serde(transparent)]
pub struct RequestId(pub u64);
