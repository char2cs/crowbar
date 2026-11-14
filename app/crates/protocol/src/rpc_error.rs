use serde::{Deserialize, Serialize};

use crate::display_text::DisplayText;
use crate::error_code::ErrorCode;

/// The error half of a `response` frame.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct RpcError {
    code: ErrorCode,
    message: DisplayText,
}

impl RpcError {
    /// A failure with `code` explained by `message`.
    #[must_use]
    pub fn new(code: ErrorCode, message: DisplayText) -> Self {
        Self { code, message }
    }

    /// Machine readable code.
    #[must_use]
    pub fn code(&self) -> ErrorCode {
        self.code
    }

    /// Human readable explanation.
    #[must_use]
    pub fn message(&self) -> &DisplayText {
        &self.message
    }
}

#[cfg(test)]
#[path = "rpc_error_tests.rs"]
mod tests;
