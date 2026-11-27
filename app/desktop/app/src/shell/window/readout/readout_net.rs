use sidebar::HostState;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum ReadoutNet {
    Ok,
    Reconnecting,
    Down,
    Repair,
    #[default]
    Idle,
}

impl ReadoutNet {
    pub(crate) fn of(state: HostState) -> Self {
        match state {
            HostState::Up => Self::Ok,
            HostState::NoLiveStream => Self::Reconnecting,
            HostState::Down | HostState::NotAvailable => Self::Down,
            HostState::NeedsRepair => Self::Repair,
            HostState::Unknown | HostState::Probing => Self::Idle,
        }
    }

    pub(crate) fn label(self) -> &'static str {
        match self {
            Self::Ok => "ok",
            Self::Reconnecting => "reconnecting",
            Self::Down => "DOWN",
            Self::Repair => "repair",
            Self::Idle => "--",
        }
    }

    pub(crate) fn warns(self) -> bool {
        matches!(self, Self::Reconnecting | Self::Down)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_host_state_reads_as_one_net_label_and_only_trouble_warns() {
        const CASES: [(HostState, &str, bool); 7] = [
            (HostState::Up, "ok", false),
            (HostState::NoLiveStream, "reconnecting", true),
            (HostState::Down, "DOWN", true),
            (HostState::NotAvailable, "DOWN", true),
            (HostState::NeedsRepair, "repair", false),
            (HostState::Unknown, "--", false),
            (HostState::Probing, "--", false),
        ];
        for (state, label, warns) in CASES {
            let net = ReadoutNet::of(state);
            assert_eq!((net.label(), net.warns()), (label, warns), "{state:?}");
        }
    }
}
