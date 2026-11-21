use startup::MigrationStep;
use store::LocalLink;
use supervisor::DaemonEvent;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum BackendStatus {
    Connected,
    Disconnected,
    Migrating(Option<MigrationStep>),
    CrashLoop(String),
}

pub(crate) fn backend_status(
    event: Option<&DaemonEvent>,
    link: LocalLink,
) -> BackendStatus {
    if link == LocalLink::Live {
        return BackendStatus::Connected;
    }
    match event {
        Some(DaemonEvent::GaveUp { report }) => BackendStatus::CrashLoop(report.clone()),
        Some(DaemonEvent::Migrating { progress }) => BackendStatus::Migrating(
            progress
                .as_ref()
                .map(|p| MigrationStep::new(p.step(), p.index(), p.total())),
        ),
        _ if link == LocalLink::Lost => BackendStatus::Disconnected,
        _ => BackendStatus::Connected,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_status_is_the_streams_unless_the_supervisor_knows_more() {
        let gave_up = DaemonEvent::GaveUp { report: "r".into() };
        let migrating = DaemonEvent::Migrating { progress: None };
        let ready = DaemonEvent::Ready { pid: None };
        let cases = [
            (Some(&gave_up), LocalLink::Live, BackendStatus::Connected),
            (
                Some(&gave_up),
                LocalLink::Lost,
                BackendStatus::CrashLoop("r".into()),
            ),
            (
                Some(&migrating),
                LocalLink::Lost,
                BackendStatus::Migrating(None),
            ),
            (Some(&ready), LocalLink::Lost, BackendStatus::Disconnected),
            (None, LocalLink::Lost, BackendStatus::Disconnected),
            (None, LocalLink::Unfollowed, BackendStatus::Connected),
        ];
        for (event, link, status) in cases {
            assert_eq!(backend_status(event, link), status, "{event:?} {link:?}");
        }
    }
}
