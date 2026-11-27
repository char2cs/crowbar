use protocol::DaemonResponse;

use crate::error::Error;

pub(crate) const DAEMON_UNAVAILABLE_CODE: &str = "daemon_unavailable";
pub(crate) const HOST_UNREACHABLE_CODE: &str = "host_unreachable";
pub(crate) const HOST_IDENTITY_CHANGED_CODE: &str = "host_identity_changed";
pub(crate) const NO_CREDENTIAL_CODE: &str = "no_credential";
pub(crate) const UNKNOWN_HOST_CODE: &str = "unknown_host";
pub(crate) const UNEXPLAINED_CODE: &str = "unexplained";

const FAILURES: [(&str, &str, &str); 5] = [
    (
        protocol::DAEMON_UNAVAILABLE,
        DAEMON_UNAVAILABLE_CODE,
        "the daemon is not running yet",
    ),
    (
        protocol::HOST_UNREACHABLE,
        HOST_UNREACHABLE_CODE,
        "the remote host is unreachable",
    ),
    (
        protocol::HOST_IDENTITY_CHANGED,
        HOST_IDENTITY_CHANGED_CODE,
        "this host's identity changed \u{2014} its certificate no longer matches what was pinned",
    ),
    (
        protocol::NO_CREDENTIAL,
        NO_CREDENTIAL_CODE,
        "no stored credential for this host \u{2014} pair it again",
    ),
    (
        protocol::UNKNOWN_HOST,
        UNKNOWN_HOST_CODE,
        "unknown remote host",
    ),
];

pub(crate) fn refusal(
    path: &str,
    response: &DaemonResponse,
) -> Option<Error> {
    let marker = response.proxy_marker()?;
    let (_, code, message) = FAILURES.iter().find(|(named, ..)| *named == marker)?;
    Some(Error::Refused {
        path: path.to_owned(),
        status: response.status(),
        code: Some((*code).to_owned()),
        message: (*message).to_owned(),
        data: Box::default(),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn marked(marker: &str) -> DaemonResponse {
        let headers = vec![(protocol::PROXY_HEADER.to_owned(), marker.to_owned())];
        DaemonResponse::new(502, headers, b"plain text".to_vec())
    }

    fn code_of(response: &DaemonResponse) -> Option<String> {
        match refusal("/p", response)? {
            Error::Refused { code, .. } => code,
            _ => None,
        }
    }

    #[test]
    fn only_a_known_marker_is_the_transports_failure() {
        assert_eq!(
            code_of(&marked(protocol::NO_CREDENTIAL)).as_deref(),
            Some(NO_CREDENTIAL_CODE)
        );
        assert_eq!(
            code_of(&marked(protocol::UNKNOWN_HOST)).as_deref(),
            Some(UNKNOWN_HOST_CODE)
        );
        assert_eq!(code_of(&marked("spoofed")), None);
        assert_eq!(
            code_of(&DaemonResponse::new(502, Vec::new(), Vec::new())),
            None
        );
    }
}
