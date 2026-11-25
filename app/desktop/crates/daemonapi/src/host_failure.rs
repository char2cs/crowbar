//! Which of its host's own troubles failed a request, as develop's `hostFailureKind` tells it.
//!
//! Spec: develop `web/src/features/hosts/lib/classify-host-failure.ts` and
//! `host-failure-notice.ts` (`carriesOnlyStatus`).

use crate::client::proxy_failure::{
    HOST_IDENTITY_CHANGED_CODE, HOST_UNREACHABLE_CODE, UNEXPLAINED_CODE, UNKNOWN_HOST_CODE,
};
use crate::error::Error;

const UNAUTHORIZED: u16 = 401;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum HostFailure {
    /// The host did not answer.
    Unreachable,
    /// The host answered with another identity than the one pinned at pairing.
    IdentityChanged,
    /// The host refuses this computer (HTTP 401): revoked, or its credential is gone.
    Refused,
    /// The host is no longer paired with this computer.
    NotPaired,
    /// The host, or the connection to it, failed with only a status and no words.
    Unexplained(u16),
}

impl HostFailure {
    #[must_use]
    pub(crate) fn of(error: &Error) -> Option<Self> {
        let Error::Refused { status, code, .. } = error else {
            return None;
        };
        match code.as_deref() {
            Some(HOST_IDENTITY_CHANGED_CODE) => Some(Self::IdentityChanged),
            Some(HOST_UNREACHABLE_CODE) => Some(Self::Unreachable),
            Some(UNKNOWN_HOST_CODE) => Some(Self::NotPaired),
            _ if *status == UNAUTHORIZED => Some(Self::Refused),
            Some(UNEXPLAINED_CODE) => Some(Self::Unexplained(*status)),
            _ => None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn refused(
        status: u16,
        code: Option<&str>,
    ) -> Error {
        Error::Refused {
            path: "/v0/x".into(),
            status,
            code: code.map(str::to_owned),
            message: "words".into(),
            data: Box::default(),
        }
    }

    #[test]
    fn each_host_trouble_is_told_from_its_code_or_status_and_a_daemon_refusal_is_none() {
        let cases = [
            (
                refused(503, Some("host_unreachable")),
                Some(HostFailure::Unreachable),
            ),
            (
                refused(502, Some("host_identity_changed")),
                Some(HostFailure::IdentityChanged),
            ),
            (
                refused(502, Some("unknown_host")),
                Some(HostFailure::NotPaired),
            ),
            (
                refused(401, Some("no_credential")),
                Some(HostFailure::Refused),
            ),
            (refused(401, None), Some(HostFailure::Refused)),
            (
                refused(500, Some("unexplained")),
                Some(HostFailure::Unexplained(500)),
            ),
            (refused(500, Some("internal")), None),
            (refused(409, Some("conflict")), None),
            (refused(500, None), None),
            (Error::Stream("closed".into()), None),
        ];
        for (error, kind) in cases {
            assert_eq!(HostFailure::of(&error), kind, "{error:?}");
        }
    }
}
