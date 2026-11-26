use ui::{ToastContent, ToastKind};

use crate::shell::window::notice_key::NoticeKey;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum DaemonNotice {
    Nothing,
    Show(NoticeKey, ToastContent),
}

const UNRESPONSIVE: &str = "Backend unresponsive";
const DUMPING: &str = "Capturing a diagnostics dump and restarting it…";
const RESTARTED: &str = "Backend restarted";
const RECONNECTS: &str = "Sessions reconnect automatically.";

pub(crate) fn daemon_notice(input: &startup::Input) -> DaemonNotice {
    match input {
        startup::Input::Restarted => {
            let restarted = ToastContent::new(ToastKind::Success, RECONNECTS).titled(RESTARTED);
            DaemonNotice::Show(NoticeKey::Restarted, restarted)
        },
        startup::Input::Unavailable(startup::Outage::Unresponsive) => {
            let wedged = ToastContent::new(ToastKind::Warning, DUMPING).titled(UNRESPONSIVE);
            DaemonNotice::Show(NoticeKey::Unresponsive, wedged)
        },
        _ => DaemonNotice::Nothing,
    }
}

#[cfg(test)]
mod tests {
    use startup::Outage;

    use super::*;

    #[test]
    fn the_supervisor_speaks_only_of_a_restart_or_a_wedge_and_a_halt_is_the_gates() {
        let restarted = ToastContent::new(ToastKind::Success, RECONNECTS).titled(RESTARTED);
        let wedged = ToastContent::new(ToastKind::Warning, DUMPING).titled(UNRESPONSIVE);
        let cases = [
            (startup::Input::Ready, DaemonNotice::Nothing),
            (
                startup::Input::Restarted,
                DaemonNotice::Show(NoticeKey::Restarted, restarted),
            ),
            (
                startup::Input::Unavailable(Outage::Unresponsive),
                DaemonNotice::Show(NoticeKey::Unresponsive, wedged),
            ),
            (
                startup::Input::Halted {
                    reason: None,
                    message: "disk full".into(),
                },
                DaemonNotice::Nothing,
            ),
            (startup::Input::Starting, DaemonNotice::Nothing),
            (
                startup::Input::Unavailable(Outage::Recovering {
                    attempt: 2,
                    reason: "exit 1".into(),
                }),
                DaemonNotice::Nothing,
            ),
            (
                startup::Input::Unavailable(Outage::GaveUp { report: "r".into() }),
                DaemonNotice::Nothing,
            ),
        ];
        for (input, notice) in cases {
            assert_eq!(daemon_notice(&input), notice, "{input:?}");
        }
    }
}
