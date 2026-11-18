use serde::{Deserialize, Serialize};

use crate::control::call::request_id::RequestId;
use crate::control::call::rpc_outcome::RpcOutcome;
use crate::control::call::rpc_response_wire::RpcResponseWire;
use crate::error::Error;

/// The answer to a `request`. On the wire exactly one of `result` and `error` is present, and a
/// `result` of JSON `null` stays a result.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(try_from = "RpcResponseWire", into = "RpcResponseWire")]
pub struct RpcResponse {
    id: RequestId,
    outcome: RpcOutcome,
}

impl RpcResponse {
    /// The answer to request `id`.
    #[must_use]
    pub fn new(
        id: RequestId,
        outcome: RpcOutcome,
    ) -> Self {
        Self { id, outcome }
    }

    /// The request being answered.
    #[must_use]
    pub fn id(&self) -> RequestId {
        self.id
    }

    /// Result or failure.
    #[must_use]
    pub fn outcome(&self) -> &RpcOutcome {
        &self.outcome
    }
}

impl TryFrom<RpcResponseWire> for RpcResponse {
    type Error = Error;

    fn try_from(wire: RpcResponseWire) -> Result<Self, Error> {
        let outcome = match (wire.result, wire.error) {
            (Some(result), None) => RpcOutcome::Ok(result),
            (None, Some(error)) => RpcOutcome::Err(error),
            (Some(_), Some(_)) => {
                return Err(Error::invalid_request(
                    "a response holds both a result and an error",
                ));
            },
            (None, None) => {
                return Err(Error::invalid_request(
                    "a response holds neither a result nor an error",
                ));
            },
        };
        Ok(Self {
            id: wire.id,
            outcome,
        })
    }
}

impl From<RpcResponse> for RpcResponseWire {
    fn from(response: RpcResponse) -> Self {
        let (result, error) = match response.outcome {
            RpcOutcome::Ok(result) => (Some(result), None),
            RpcOutcome::Err(error) => (None, Some(error)),
        };
        Self {
            id: response.id,
            result,
            error,
        }
    }
}

#[cfg(test)]
mod tests {
    use serde_json::{Value, json};

    use super::*;
    use crate::control::call::error_code::ErrorCode;
    use crate::control::call::rpc_error::RpcError;
    use crate::control::payload::DisplayText;

    #[test]
    fn a_null_result_survives_a_round_trip_as_a_result() {
        let response = RpcResponse::new(RequestId::new(1), RpcOutcome::Ok(Value::Null));
        let text = serde_json::to_string(&response).unwrap();
        assert_eq!(text, r#"{"id":1,"result":null}"#, "null is written");
        assert_eq!(
            serde_json::from_str::<RpcResponse>(&text).unwrap(),
            response,
            "and read back"
        );
    }

    #[test]
    fn an_error_round_trips() {
        let error = RpcError::new(ErrorCode::Rejected, DisplayText::new("no").unwrap());
        let response = RpcResponse::new(RequestId::new(2), RpcOutcome::Err(error));
        let text = serde_json::to_string(&response).unwrap();
        assert_eq!(
            text, r#"{"id":2,"error":{"code":"Rejected","message":"no"}}"#,
            "shape"
        );
        assert_eq!(
            serde_json::from_str::<RpcResponse>(&text).unwrap(),
            response,
            "round trip"
        );
        assert_eq!(response.id(), RequestId::new(2), "id");
        assert!(matches!(response.outcome(), RpcOutcome::Err(_)), "outcome");
    }

    #[test]
    fn both_or_neither_is_refused_on_the_wire() {
        let cases = [
            json!({"id": 1}),
            json!({"id": 1, "result": 1, "error": {"code": "Timeout", "message": ""}}),
            json!({"id": 1, "result": null, "error": {"code": "Timeout", "message": ""}}),
        ];
        for wire in cases {
            assert!(
                serde_json::from_value::<RpcResponse>(wire.clone()).is_err(),
                "{wire}"
            );
        }
    }
}
