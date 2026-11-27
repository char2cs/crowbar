use serde_json::Value;

use crate::control::call::rpc_error::RpcError;

#[derive(Debug, Clone, PartialEq)]
pub enum RpcOutcome {
    Ok(Value),
    Err(RpcError),
}
