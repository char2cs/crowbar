use serde::{Deserialize, Serialize};

/// Error codes a request can fail with, shared with the host protocol.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum ErrorCode {
    /// The caller's tier does not allow the call.
    PermissionDenied,
    /// The extension backend is not running yet.
    NotActivated,
    /// The backend or daemon cannot be reached.
    Unavailable,
    /// The request exceeded its time budget.
    Timeout,
    /// The parameters failed schema validation.
    InvalidParams,
    /// Core validation refused a command.
    Rejected,
}
