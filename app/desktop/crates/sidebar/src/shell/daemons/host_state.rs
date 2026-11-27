use crate::shell::daemons::host_tone::HostTone;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum HostState {
    Up,
    NoLiveStream,
    Down,
    NotAvailable,
    NeedsRepair,
    Unknown,
    Probing,
}

impl HostState {
    pub fn tone(self) -> HostTone {
        match self {
            Self::Up | Self::NoLiveStream => HostTone::Online,
            Self::Down | Self::NotAvailable => HostTone::Offline,
            Self::NeedsRepair => HostTone::NeedsAttention,
            Self::Unknown | Self::Probing => HostTone::Unsettled,
        }
    }

    pub fn label(self) -> &'static str {
        match self {
            Self::Unknown => "Status unknown",
            Self::Probing => "Checking…",
            Self::Up => "Online",
            Self::Down => "Offline",
            Self::NeedsRepair => "Needs pairing again",
            Self::NoLiveStream => "Online — no live stream",
            Self::NotAvailable => "Host not available",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_state_maps_to_its_tone_and_label() {
        let cases = [
            (HostState::Up, HostTone::Online, "Online"),
            (
                HostState::NoLiveStream,
                HostTone::Online,
                "Online — no live stream",
            ),
            (HostState::Down, HostTone::Offline, "Offline"),
            (
                HostState::NotAvailable,
                HostTone::Offline,
                "Host not available",
            ),
            (
                HostState::NeedsRepair,
                HostTone::NeedsAttention,
                "Needs pairing again",
            ),
            (HostState::Unknown, HostTone::Unsettled, "Status unknown"),
            (HostState::Probing, HostTone::Unsettled, "Checking…"),
        ];
        for (state, tone, label) in cases {
            assert_eq!(state.tone(), tone, "{state:?} tone");
            assert_eq!(state.label(), label, "{state:?} label");
        }
    }
}
