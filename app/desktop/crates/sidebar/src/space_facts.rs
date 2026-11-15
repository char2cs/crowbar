//! Whether the create-space panel may create, and why not.

use crate::host::Host;
use crate::host_kind::HostKind;
use crate::host_tone::HostTone;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct SpaceFacts {
    pub(crate) can_create: bool,
    pub(crate) hint: Option<String>,
}

impl SpaceFacts {
    pub(crate) fn of(
        host: &Host,
        folder: &str,
    ) -> Self {
        let name = &host.name;
        let remote = host.kind == HostKind::Remote;
        let hint = match host.state.tone() {
            HostTone::NeedsAttention if remote => Some(format!("{name} needs pairing again.")),
            HostTone::Offline if remote => Some(format!("{name} is not answering.")),
            _ if remote && folder.trim().is_empty() => {
                Some(format!("Choose a folder on {name} first."))
            },
            _ => None,
        };
        let reachable = !remote || host.state.tone() == HostTone::Online;
        Self {
            can_create: reachable && !folder.trim().is_empty(),
            hint,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::host_id::HostId;
    use crate::host_state::HostState;

    fn host(
        kind: HostKind,
        state: HostState,
    ) -> Host {
        Host {
            id: HostId::from("h"),
            name: "devbox".into(),
            kind,
            state,
            endpoint: None,
        }
    }

    type Case = (
        &'static str,
        HostKind,
        HostState,
        &'static str,
        bool,
        Option<&'static str>,
    );

    const CASES: [Case; 6] = [
        (
            "local with folder",
            HostKind::Local,
            HostState::Up,
            "/w",
            true,
            None,
        ),
        (
            "local without folder",
            HostKind::Local,
            HostState::Up,
            " ",
            false,
            None,
        ),
        (
            "remote without folder",
            HostKind::Remote,
            HostState::Up,
            "",
            false,
            Some("Choose a folder on devbox first."),
        ),
        (
            "remote needing repair",
            HostKind::Remote,
            HostState::NeedsRepair,
            "/w",
            false,
            Some("devbox needs pairing again."),
        ),
        (
            "remote down",
            HostKind::Remote,
            HostState::Down,
            "/w",
            false,
            Some("devbox is not answering."),
        ),
        (
            "remote ready",
            HostKind::Remote,
            HostState::Up,
            "/w",
            true,
            None,
        ),
    ];

    #[test]
    fn creation_needs_a_folder_and_a_reachable_host() {
        for (name, kind, state, folder, can_create, hint) in CASES {
            let facts = SpaceFacts::of(&host(kind, state), folder);
            assert_eq!(facts.can_create, can_create, "{name}");
            assert_eq!(facts.hint.as_deref(), hint, "{name}");
        }
    }
}
