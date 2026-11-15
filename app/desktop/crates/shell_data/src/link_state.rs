//! Whether a host's stream is up.

use sidebar::HostState;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum LinkState {
    /// Opening, or reopening after a drop.
    #[default]
    Connecting,
    /// Its `ready` frame has arrived.
    Live,
    /// The socket is gone and the next attempt waits.
    Down,
}

impl LinkState {
    pub(crate) fn host_state(self) -> HostState {
        match self {
            Self::Connecting => HostState::Probing,
            Self::Live => HostState::Up,
            Self::Down => HostState::Down,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_link_state_has_a_host_state() {
        assert_eq!(LinkState::Connecting.host_state(), HostState::Probing);
        assert_eq!(LinkState::Live.host_state(), HostState::Up);
        assert_eq!(LinkState::Down.host_state(), HostState::Down);
    }
}
