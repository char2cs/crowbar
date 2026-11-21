//! How the shell's stream to its own daemon stands, as develop's connection store tells it.

use crate::live::{LinkState, State};
use crate::store::Store;

/// The shell's stream to the daemon on this computer.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LocalLink {
    /// No stream to this computer's daemon is followed.
    Unfollowed,
    /// The stream is open.
    Live,
    /// The stream is closed and being opened again.
    Lost,
}

impl Store {
    /// How the stream to this computer's daemon stands now.
    pub fn local_link(&self) -> LocalLink {
        self.hub.read(link_of)
    }
}

fn link_of(state: &State) -> LocalLink {
    let local = state.hosts.iter().find(|slot| slot.spec.endpoint.is_none());
    match local.map(|slot| slot.link) {
        None => LocalLink::Unfollowed,
        Some(LinkState::Live) => LocalLink::Live,
        Some(LinkState::Connecting | LinkState::Down) => LocalLink::Lost,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    #[test]
    fn a_stream_that_is_not_open_is_lost_until_it_opens_again() {
        let cases = [
            (LinkState::Live, LocalLink::Live),
            (LinkState::Connecting, LocalLink::Lost),
            (LinkState::Down, LocalLink::Lost),
        ];
        for (link, expected) in cases {
            let (mut state, _daemon) = state();
            state.hosts[0].link = link;
            assert_eq!(link_of(&state), expected, "{link:?}");
        }
        let (mut alone, _daemon) = state();
        alone.hosts.clear();
        assert_eq!(link_of(&alone), LocalLink::Unfollowed);
    }
}
