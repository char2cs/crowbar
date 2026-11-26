//! The host's side of the daemon connection.

use async_channel::{Receiver, Sender};
use terminal::{ClientMessage, DaemonFrame};

/// Where the host puts frames read from the PTY websocket and takes messages to write to it.
#[derive(Debug, Clone)]
pub struct Peer {
    frames: Sender<DaemonFrame>,
    messages: Receiver<ClientMessage>,
}

impl Peer {
    pub(crate) fn new(
        frames: Sender<DaemonFrame>,
        messages: Receiver<ClientMessage>,
    ) -> Self {
        Self { frames, messages }
    }

    /// The raw channel ends, for a host that pumps them from its own tasks.
    #[must_use]
    pub fn into_channels(self) -> (Sender<DaemonFrame>, Receiver<ClientMessage>) {
        (self.frames, self.messages)
    }
}

#[cfg(test)]
mod tests {
    use crate::daemon::link::Link;

    use super::*;

    #[test]
    fn a_dropped_view_ends_both_directions() {
        let (link, peer) = Link::open();
        let (frames, messages) = peer.into_channels();
        assert!(frames.send_blocking(DaemonFrame::Exit(1)).is_ok());
        drop(link);
        assert!(frames.send_blocking(DaemonFrame::Exit(2)).is_err());
        assert!(messages.recv_blocking().is_err());
    }
}
