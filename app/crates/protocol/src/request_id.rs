use serde::{Deserialize, Serialize};

/// Correlates a `request` or `subscribe` frame with its answer.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
#[serde(transparent)]
pub struct RequestId(u64);

impl RequestId {
    /// The id with number `number`.
    #[must_use]
    pub fn new(number: u64) -> Self {
        Self(number)
    }

    /// The id's number.
    #[must_use]
    pub fn get(self) -> u64 {
        self.0
    }
}

#[cfg(test)]
#[path = "request_id_tests.rs"]
mod tests;
