use startup::Outage;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum DaemonNotice {
    Clear,
    Show(String),
}

const UNAVAILABLE: &str = "Backend unavailable";

pub(crate) fn daemon_notice(input: &startup::Input) -> DaemonNotice {
    match input {
        startup::Input::Ready => DaemonNotice::Clear,
        startup::Input::Starting | startup::Input::Migrating { .. } => {
            DaemonNotice::Show(format!("{UNAVAILABLE}: starting again"))
        },
        startup::Input::Halted { message, .. } => {
            DaemonNotice::Show(format!("{UNAVAILABLE}: {message}"))
        },
        startup::Input::Unavailable(outage) => DaemonNotice::Show(outage_text(outage)),
        _ => DaemonNotice::Show(UNAVAILABLE.to_owned()),
    }
}

fn outage_text(outage: &Outage) -> String {
    match outage {
        Outage::Unresponsive => format!("{UNAVAILABLE}: not responding"),
        Outage::Recovering { attempt, .. } => {
            format!("{UNAVAILABLE}: restarting (attempt {attempt})")
        },
        Outage::GaveUp { .. } => format!("{UNAVAILABLE}: gave up restarting it"),
        Outage::Stopped => format!("{UNAVAILABLE}: stopped"),
        _ => UNAVAILABLE.to_owned(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn text(input: &startup::Input) -> String {
        match daemon_notice(input) {
            DaemonNotice::Show(text) => text,
            DaemonNotice::Clear => String::new(),
        }
    }

    #[test]
    fn ready_clears_the_notice() {
        assert_eq!(
            daemon_notice(&startup::Input::Ready),
            DaemonNotice::Clear,
            "ready"
        );
    }

    #[test]
    fn every_outage_says_the_backend_is_unavailable() {
        let cases = [
            startup::Input::Unavailable(Outage::Unresponsive),
            startup::Input::Unavailable(Outage::Recovering {
                attempt: 2,
                reason: "exit 1".into(),
            }),
            startup::Input::Unavailable(Outage::GaveUp { report: "r".into() }),
            startup::Input::Unavailable(Outage::Stopped),
            startup::Input::Halted {
                reason: None,
                message: "disk full".into(),
            },
            startup::Input::Starting,
        ];
        for input in cases {
            assert!(text(&input).starts_with("Backend unavailable"), "{input:?}");
        }
    }

    #[test]
    fn a_recovering_daemon_names_the_attempt() {
        let input = startup::Input::Unavailable(Outage::Recovering {
            attempt: 3,
            reason: "x".into(),
        });
        assert!(text(&input).contains("attempt 3"), "attempt");
    }
}
