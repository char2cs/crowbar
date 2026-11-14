use serde::{Deserialize, Serialize};

use crate::error_code::ErrorCode;

/// The error half of a `response` frame.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct RpcError {
    /// Machine readable code.
    pub code: ErrorCode,
    /// Human readable explanation.
    pub message: String,
}
