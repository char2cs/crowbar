use serde::{Deserialize, Deserializer, Serialize};
use serde_json::Value;

use crate::request_id::RequestId;
use crate::rpc_error::RpcError;

fn present<'de, D: Deserializer<'de>>(deserializer: D) -> Result<Option<Value>, D::Error> {
    Value::deserialize(deserializer).map(Some)
}

#[derive(Serialize, Deserialize)]
pub(crate) struct RpcResponseWire {
    pub(crate) id: RequestId,
    #[serde(
        default,
        deserialize_with = "present",
        skip_serializing_if = "Option::is_none"
    )]
    pub(crate) result: Option<Value>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub(crate) error: Option<RpcError>,
}

#[cfg(test)]
#[path = "rpc_response_wire_tests.rs"]
mod tests;
