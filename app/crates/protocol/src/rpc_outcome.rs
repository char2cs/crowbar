use serde_json::Value;

use crate::rpc_error::RpcError;

/// How a request ended: with a result (which may be JSON `null`) or with an error, never both
/// and never neither.
#[derive(Debug, Clone, PartialEq)]
pub enum RpcOutcome {
    /// The call succeeded.
    Ok(Value),
    /// The call failed.
    Err(RpcError),
}
