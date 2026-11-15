use std::fmt;

use tauri::http;

use crate::api_proxy::proxy_answer::proxy_error;
use crate::api_proxy::{HOST_IDENTITY_CHANGED, HOST_UNREACHABLE};

pub(crate) enum DialError {
    Unreachable(String),
    IdentityChanged(String),
}

impl DialError {
    pub(crate) fn marker(&self) -> &'static str {
        match self {
            Self::Unreachable(_) => HOST_UNREACHABLE,
            Self::IdentityChanged(_) => HOST_IDENTITY_CHANGED,
        }
    }

    pub(crate) fn into_response(self) -> http::Response<Vec<u8>> {
        let status = match self {
            Self::Unreachable(_) => http::StatusCode::SERVICE_UNAVAILABLE,
            Self::IdentityChanged(_) => http::StatusCode::BAD_GATEWAY,
        };
        proxy_error(status, self.marker(), &self.to_string())
    }
}

impl fmt::Display for DialError {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Unreachable(msg) | Self::IdentityChanged(msg) => f.write_str(msg),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_failure_answers_with_its_status_marker_and_message() {
        let cases = [
            (DialError::Unreachable("down".into()), 503, HOST_UNREACHABLE),
            (
                DialError::IdentityChanged("swapped".into()),
                502,
                HOST_IDENTITY_CHANGED,
            ),
        ];
        for (error, status, marker) in cases {
            let message = error.to_string();
            assert_eq!(error.marker(), marker);

            let resp = error.into_response();

            assert_eq!(resp.status(), status, "{message}");
            assert_eq!(resp.headers().get("x-crowbar-proxy").unwrap(), marker);
            assert_eq!(resp.body().as_slice(), message.as_bytes());
        }
    }
}
