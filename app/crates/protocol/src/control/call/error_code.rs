use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum ErrorCode {
    PermissionDenied,
    NotActivated,
    Unavailable,
    Timeout,
    InvalidParams,
    Rejected,
}
