//! The `{success, data, error, code}` envelope every daemon route answers with.

use protocol::DaemonResponse;
use serde::Deserialize;
use serde_json::Value;

use crate::client::proxy_failure;
use crate::error::{Error, Result};

/// The `{success, data, error, code}` envelope a daemon route answers with.
///
/// [`Client::send`](crate::Client::send) opens it; a caller that sends its own requests opens
/// the answer with [`Envelope::unwrap`].
#[derive(Debug, Deserialize)]
pub struct Envelope {
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

impl Envelope {
    /// The `data` of a daemon answer to `path`, or why the daemon refused.
    ///
    /// An empty body with a success status reads as `null`.
    ///
    /// # Errors
    ///
    /// [`Error::Refused`] when the status is an error or the envelope says `success: false` (with
    /// the daemon's message when it sent one), and [`Error::Body`] when a success body is not an
    /// envelope.
    pub fn unwrap(
        path: &str,
        response: &DaemonResponse,
    ) -> Result<Value> {
        if let Some(refusal) = proxy_failure::refusal(path, response) {
            return Err(refusal);
        }
        let status = response.status();
        if response.body().is_empty() && status < FIRST_ERROR_STATUS {
            return Ok(Value::Null);
        }
        let parsed = serde_json::from_slice::<Self>(response.body());
        match parsed {
            Ok(envelope) if envelope.success && status < FIRST_ERROR_STATUS => Ok(envelope.data),
            Ok(envelope) => Err(refused(path, status, envelope)),
            Err(source) if status < FIRST_ERROR_STATUS => Err(Error::Body {
                path: path.to_owned(),
                source,
            }),
            Err(unparsed) => Err(unparsed_refusal(path, status, &unparsed)),
        }
    }
}

fn unparsed_refusal(
    path: &str,
    status: u16,
    unparsed: &serde_json::Error,
) -> Error {
    tracing::debug!("{path}: the error body is not an envelope: {unparsed}");
    Error::Refused {
        path: path.to_owned(),
        status,
        code: Some(proxy_failure::UNEXPLAINED_CODE.to_owned()),
        message: format!("the daemon answered {status}"),
        data: Box::default(),
    }
}

fn refused(
    path: &str,
    status: u16,
    envelope: Envelope,
) -> Error {
    // SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/hosts/lib/host-failure-notice.ts
    let worded = envelope.error.is_some() || envelope.code.is_some();
    let code = envelope
        .code
        .or_else(|| (!worded).then(|| proxy_failure::UNEXPLAINED_CODE.to_owned()));
    Error::Refused {
        path: path.to_owned(),
        status,
        code,
        message: envelope
            .error
            .unwrap_or_else(|| format!("the daemon answered {status}")),
        data: Box::new(envelope.data),
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
                data,
                ..
            }) => format!(
                "refused {status} {} {message} {data}",
                code.unwrap_or_default()
            ),
            Err(Error::Body { .. }) => "body".to_owned(),
            Err(other) => other.to_string(),
        }
    }

    const CASES: [(u16, &str, &str); 7] = [
        (
            200,
            r#"{"success":true,"data":{"id":"a"}}"#,
            r#"ok {"id":"a"}"#,
        ),
        (204, "", "ok null"),
        (
            409,
            r#"{"success":false,"error":"name taken","code":"conflict"}"#,
            "refused 409 conflict name taken null",
        ),
        (
            409,
            r#"{"success":false,"error":"lose work","code":"work_at_risk","data":{"workAtRisk":[]}}"#,
            r#"refused 409 work_at_risk lose work {"workAtRisk":[]}"#,
        ),
        (
            502,
            "<html>",
            "refused 502 unexplained the daemon answered 502 null",
        ),
        (200, "nope", "body"),
        (
            429,
            r#"{"success":false}"#,
            "refused 429 unexplained the daemon answered 429 null",
        ),
    ];

    #[test]
    fn each_answer_reads_as_its_data_its_refusal_or_its_failure() {
        for (status, body, expected) in CASES {
            assert_eq!(
                shown(Envelope::unwrap("/p", &answer(status, body))),
                expected,
                "{status} {body}"
            );
        }
    }

    #[test]
    fn a_proxy_answer_reads_as_its_marked_failure_and_an_unworded_one_as_unexplained() {
        let marked = |marker: &str| {
            let headers = vec![(protocol::PROXY_HEADER.to_owned(), marker.to_owned())];
            DaemonResponse::new(503, headers, b"cannot reach 10.0.0.1:4747".to_vec())
        };
        assert_eq!(
            shown(Envelope::unwrap("/p", &marked(protocol::HOST_UNREACHABLE))),
            "refused 503 host_unreachable the remote host is unreachable null"
        );
        assert_eq!(
            shown(Envelope::unwrap(
                "/p",
                &marked(protocol::HOST_IDENTITY_CHANGED)
            )),
            "refused 503 host_identity_changed this host's identity changed \u{2014} its certificate no longer matches what was pinned null"
        );
        assert_eq!(
            shown(Envelope::unwrap("/p", &answer(502, "crowbar proxy error"))),
            "refused 502 unexplained the daemon answered 502 null"
        );
    }
}
