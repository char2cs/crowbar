use serde::{Deserialize, Serialize};

use crate::error::Error;
use crate::request_id::RequestId;
use crate::rpc_outcome::RpcOutcome;
use crate::rpc_response_wire::RpcResponseWire;

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
    pub fn new(id: RequestId, outcome: RpcOutcome) -> Self {
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
                return Err(Error::InvalidRequest {
                    reason: "a response holds both a result and an error",
                });
            },
            (None, None) => {
                return Err(Error::InvalidRequest {
                    reason: "a response holds neither a result nor an error",
                });
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
#[path = "rpc_response_tests.rs"]
mod tests;
