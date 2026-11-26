use startup::MigrationStep;
use ui::{ToastContent, ToastKind, ToastLifetime};

use crate::daemon::backend::backend_status::BackendStatus;
use crate::daemon::backend::notice_act::NoticeAct;
use crate::daemon::backend::warned::{Reach, Warned};

const STARTING: &str = "Crowbar is starting";
const UPGRADING: &str = "Crowbar is upgrading its data";
const UNAVAILABLE: &str = "Backend unavailable";
const RECONNECTING: &str = "Reconnecting…";
const RECONNECTED: &str = "Reconnected";
const RESTORED: &str = "Backend connection restored.";

#[derive(Debug, Default)]
pub(crate) struct ConnectionNotice {
    followed: Option<BackendStatus>,
    warned: Warned,
    reach: Reach,
}

impl ConnectionNotice {
    pub(crate) fn follow(
        &mut self,
        status: BackendStatus,
    ) -> NoticeAct {
        if self.followed.as_ref() == Some(&status) {
            return NoticeAct::Keep;
        }
        self.followed = Some(status.clone());
        match status {
            BackendStatus::CrashLoop(_) => self.withdraw(),
            BackendStatus::Migrating(step) => self.show(migrating(step.as_ref())),
            BackendStatus::Connected => self.recover(),
            BackendStatus::Disconnected => NoticeAct::WarnLater,
        }
    }

    pub(crate) fn warn(&mut self) -> (ToastContent, ToastLifetime) {
        self.warned = Warned::Yes;
        let warning = ToastContent::new(ToastKind::Warning, RECONNECTING).titled(UNAVAILABLE);
        (warning, ToastLifetime::Pinned)
    }

    fn withdraw(&mut self) -> NoticeAct {
        self.warned = Warned::No;
        NoticeAct::Dismiss
    }

    fn show(
        &mut self,
        content: ToastContent,
    ) -> NoticeAct {
        self.warned = Warned::Yes;
        NoticeAct::Show(content, ToastLifetime::Pinned)
    }

    fn recover(&mut self) -> NoticeAct {
        let reconnecting = self.reach == Reach::Reached;
        self.reach = Reach::Reached;
        if self.warned == Warned::No {
            return NoticeAct::Settle;
        }
        self.warned = Warned::No;
        if !reconnecting {
            return NoticeAct::Dismiss;
        }
        let back = ToastContent::new(ToastKind::Success, RESTORED).titled(RECONNECTED);
        NoticeAct::Renew(back, ToastLifetime::Auto)
    }
}

fn migrating(step: Option<&MigrationStep>) -> ToastContent {
    let Some(step) = step else {
        return ToastContent::new(ToastKind::Info, STARTING);
    };
    ToastContent::new(ToastKind::Info, step.label()).titled(UPGRADING)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn acts(statuses: &[BackendStatus]) -> Vec<NoticeAct> {
        let mut notice = ConnectionNotice::default();
        statuses
            .iter()
            .map(|status| step(&mut notice, status))
            .collect()
    }

    fn step(
        notice: &mut ConnectionNotice,
        status: &BackendStatus,
    ) -> NoticeAct {
        let act = notice.follow(status.clone());
        if act == NoticeAct::WarnLater {
            notice.warn();
        }
        act
    }

    fn reconnected() -> NoticeAct {
        let back = ToastContent::new(ToastKind::Success, RESTORED).titled(RECONNECTED);
        NoticeAct::Renew(back, ToastLifetime::Auto)
    }

    type Case<'a> = (&'a [BackendStatus], Vec<NoticeAct>);

    fn assert_cases(cases: Vec<Case<'_>>) {
        for (statuses, expected) in cases {
            assert_eq!(acts(statuses), expected, "{statuses:?}");
        }
    }

    fn starting() -> NoticeAct {
        let content = ToastContent::new(ToastKind::Info, STARTING);
        NoticeAct::Show(content, ToastLifetime::Pinned)
    }

    #[test]
    fn an_outage_warns_after_a_moment_and_its_end_says_reconnected_once() {
        use BackendStatus::{Connected, Disconnected, Migrating};
        assert_cases(vec![
            (&[Connected], vec![NoticeAct::Settle]),
            (
                &[Connected, Disconnected, Disconnected, Connected],
                vec![
                    NoticeAct::Settle,
                    NoticeAct::WarnLater,
                    NoticeAct::Keep,
                    reconnected(),
                ],
            ),
            (
                &[Connected, Migrating(None), Connected],
                vec![NoticeAct::Settle, starting(), reconnected()],
            ),
        ]);
    }

    #[test]
    fn the_launch_never_says_reconnected() {
        use BackendStatus::{Connected, CrashLoop, Disconnected, Migrating};
        assert_cases(vec![
            (
                &[Disconnected, CrashLoop("r".into()), Connected],
                vec![NoticeAct::WarnLater, NoticeAct::Dismiss, NoticeAct::Settle],
            ),
            (
                &[Migrating(None), Connected],
                vec![starting(), NoticeAct::Dismiss],
            ),
            (
                &[Disconnected, Connected],
                vec![NoticeAct::WarnLater, NoticeAct::Dismiss],
            ),
        ]);
    }

    #[test]
    fn a_migration_step_is_named_under_the_upgrade_title() {
        let step = MigrationStep::new("index chats", 2, 5);
        let content = migrating(Some(&step));
        let expected =
            ToastContent::new(ToastKind::Info, "Step 2 of 5: index chats").titled(UPGRADING);
        assert_eq!(content, expected);
    }
}
