use crate::error::Error;
use crate::host_failure::HostFailure;

const PAIR_AGAIN: &str = "To use it again, remove it here, then add it back in Settings \u{2192} Hosts with a new code from Settings \u{2192} Devices on that host (or crowbar pair on a headless host).";

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct HostNotice {
    pub message: String,
    pub description: Option<String>,
}

impl HostNotice {
    #[must_use]
    pub fn of(
        error: &Error,
        name: Option<&str>,
    ) -> Option<Self> {
        let failure = HostFailure::of(error)?;
        let Some(name) = name.filter(|_| failure != HostFailure::NotPaired) else {
            return Some(Self::host_not_paired());
        };
        Some(Self::told(failure, name))
    }

    fn host_not_paired() -> Self {
        Self::described(
            "That host is no longer paired with this computer",
            "Pair it again in Settings \u{2192} Hosts to use it.",
        )
    }

    #[must_use]
    pub fn unreachable(name: &str) -> Self {
        Self::described(
            &format!("Couldn\u{2019}t reach {name}"),
            "Check that it is online in Settings \u{2192} Hosts.",
        )
    }

    fn told(
        failure: HostFailure,
        name: &str,
    ) -> Self {
        match failure {
            HostFailure::Unreachable => Self::unreachable(name),
            HostFailure::IdentityChanged => Self::described(
                &format!(
                    "{name} is presenting a different identity \u{2014} confirm it before pairing again"
                ),
                "It may have been reinstalled or reset, or someone may be intercepting the connection, so Crowbar refused to connect. Only pair it again after you confirm its new fingerprint with whoever runs it: remove it here, then add it back in Settings \u{2192} Hosts with a new code from Settings \u{2192} Devices on that host (or crowbar pair on a headless host).",
            ),
            HostFailure::Refused => Self::described(
                &format!("{name} needs to be paired again"),
                &format!(
                    "It doesn\u{2019}t accept this computer\u{2019}s credential, or none is stored for it here. {PAIR_AGAIN}"
                ),
            ),
            HostFailure::Unexplained(status) => Self::described(
                &format!("{name} returned an error ({status})"),
                "It, or the connection to it, failed this request. Try again.",
            ),
            HostFailure::NotPaired => Self::host_not_paired(),
        }
    }

    fn described(
        message: &str,
        description: &str,
    ) -> Self {
        Self {
            message: message.to_owned(),
            description: Some(description.to_owned()),
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
            message: "client is not paired".into(),
            data: Box::default(),
        }
    }

    fn headline(
        error: &Error,
        name: Option<&str>,
    ) -> Option<String> {
        HostNotice::of(error, name).map(|notice| notice.message)
    }

    #[test]
    fn each_host_trouble_is_told_in_develops_words_by_the_hosts_name() {
        let devbox = Some("devbox");
        let cases = [
            (
                refused(503, Some("host_unreachable")),
                "Couldn\u{2019}t reach devbox",
            ),
            (
                refused(502, Some("host_identity_changed")),
                "devbox is presenting a different identity \u{2014} confirm it before pairing again",
            ),
            (refused(401, None), "devbox needs to be paired again"),
            (
                refused(502, Some("unexplained")),
                "devbox returned an error (502)",
            ),
            (
                refused(502, Some("unknown_host")),
                "That host is no longer paired with this computer",
            ),
        ];
        for (error, words) in cases {
            assert_eq!(
                headline(&error, devbox).as_deref(),
                Some(words),
                "{error:?}"
            );
        }
    }

    #[test]
    fn a_removed_host_has_no_name_and_a_daemon_refusal_is_not_the_hosts() {
        assert_eq!(
            headline(&refused(503, Some("host_unreachable")), None).as_deref(),
            Some("That host is no longer paired with this computer")
        );
        assert_eq!(
            headline(&refused(409, Some("conflict")), Some("devbox")),
            None
        );
        let unreachable = HostNotice::unreachable("devbox");
        assert_eq!(
            unreachable.description.as_deref(),
            Some("Check that it is online in Settings \u{2192} Hosts.")
        );
    }
}
