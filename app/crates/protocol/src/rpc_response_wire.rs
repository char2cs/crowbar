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
mod tests {
    use serde_json::{Value, json};

    use super::*;

    #[test]
    fn a_present_null_is_some_and_an_absent_key_is_none() {
        let null: RpcResponseWire =
            serde_json::from_value(json!({"id": 1, "result": null})).unwrap();
        assert_eq!(null.result, Some(Value::Null), "null is present");
        let absent: RpcResponseWire = serde_json::from_value(json!({"id": 1})).unwrap();
        assert_eq!(absent.result, None, "absent");
    }
}
