//! The `{success, data, error, code}` envelope every daemon route answers with.

use protocol::DaemonResponse;
use serde::Deserialize;
use serde_json::Value;

use crate::error::{Error, Result};

#[derive(Deserialize)]
struct Envelope {
    #[serde(default)]
    success: bool,
    #[serde(default)]
    data: Value,
    #[serde(default)]
    error: Option<String>,
    #[serde(default)]
    code: Option<String>,
}

pub(crate) const FIRST_ERROR_STATUS: u16 = 400;

pub(crate) fn unwrap(
    path: &str,
    response: &DaemonResponse,
) -> Result<Value> {
    let status = response.status();
    if response.body().is_empty() && status < FIRST_ERROR_STATUS {
        return Ok(Value::Null);
    }
    let parsed = serde_json::from_slice::<Envelope>(response.body());
    match parsed {
        Ok(envelope) if envelope.success && status < FIRST_ERROR_STATUS => Ok(envelope.data),
        Ok(envelope) => Err(refused(path, status, envelope)),
        Err(source) if status < FIRST_ERROR_STATUS => Err(Error::Body {
            path: path.to_owned(),
            source,
        }),
        Err(_) => Err(Error::Refused {
            path: path.to_owned(),
            status,
            code: None,
            message: format!("the daemon answered {status}"),
        }),
    }
}

fn refused(
    path: &str,
    status: u16,
    envelope: Envelope,
) -> Error {
    Error::Refused {
        path: path.to_owned(),
        status,
        code: envelope.code,
        message: envelope
            .error
            .unwrap_or_else(|| format!("the daemon answered {status}")),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn answer(
        status: u16,
        body: &str,
    ) -> DaemonResponse {
        DaemonResponse::new(status, Vec::new(), body.as_bytes().to_vec())
    }

    fn shown(result: Result<Value>) -> String {
        match result {
            Ok(data) => format!("ok {data}"),
            Err(Error::Refused {
                status,
                code,
                message,
                ..
            }) => format!("refused {status} {} {message}", code.unwrap_or_default()),
            Err(Error::Body { .. }) => "body".to_owned(),
            Err(other) => other.to_string(),
        }
    }

    const CASES: [(u16, &str, &str); 5] = [
        (
            200,
            r#"{"success":true,"data":{"id":"a"}}"#,
            r#"ok {"id":"a"}"#,
        ),
        (204, "", "ok null"),
        (
            409,
            r#"{"success":false,"error":"name taken","code":"conflict"}"#,
            "refused 409 conflict name taken",
        ),
        (502, "<html>", "refused 502  the daemon answered 502"),
        (200, "nope", "body"),
    ];

    #[test]
    fn each_answer_reads_as_its_data_its_refusal_or_its_failure() {
        for (status, body, expected) in CASES {
            assert_eq!(
                shown(unwrap("/p", &answer(status, body))),
                expected,
                "{status} {body}"
            );
        }
    }
}
