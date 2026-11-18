//! Connection state of a daemon host.

use crate::shell::daemons::host_tone::HostTone;

/// What the shell knows about a host's daemon.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum HostState {
    /// Reachable with a live stream.
    Up,
    /// Reachable, but no live stream.
    NoLiveStream,
    /// Not reachable.
    Down,
    /// The host answered that it is not available.
    NotAvailable,
    /// Pairing must be redone.
    NeedsRepair,
    /// Not known yet.
    Unknown,
    /// A probe is in flight.
    Probing,
}

impl HostState {
    /// The colour family the state is shown in.
    pub fn tone(self) -> HostTone {
        match self {
            Self::Up | Self::NoLiveStream => HostTone::Online,
            Self::Down | Self::NotAvailable => HostTone::Offline,
            Self::NeedsRepair => HostTone::NeedsAttention,
            Self::Unknown | Self::Probing => HostTone::Unsettled,
        }
    }

    /// The label of the state in tooltips and menus.
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
