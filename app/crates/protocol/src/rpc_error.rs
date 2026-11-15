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
    pub fn new(
        code: ErrorCode,
        message: DisplayText,
    ) -> Self {
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
mod tests {
    use super::*;

    #[test]
    fn carries_a_code_and_bounded_text() {
        let error = RpcError::new(ErrorCode::Timeout, DisplayText::new("too slow").unwrap());
        assert_eq!(error.code(), ErrorCode::Timeout, "code");
        assert_eq!(error.message().as_str(), "too slow", "message");
        let long = format!(r#"{{"code":"Timeout","message":"{}"}}"#, "x".repeat(2000));
        assert!(
            serde_json::from_str::<RpcError>(&long).is_err(),
            "message is capped on the wire"
        );
    }
}
