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

    /// Hands a frame from the daemon to the view, for a host that reads the socket on its own
    /// thread; false once the view is gone.
    pub fn deliver_blocking(
        &self,
        frame: DaemonFrame,
    ) -> bool {
        self.frames.send_blocking(frame).is_ok()
    }

    /// The next message the view wants sent to the daemon, for a host that writes the socket on
    /// its own thread; `None` once the view is gone.
    pub fn next_message_blocking(&self) -> Option<ClientMessage> {
        self.messages.recv_blocking().ok()
    }

    /// The next message if one is already waiting.
    pub fn try_next_message(&self) -> Option<ClientMessage> {
        self.messages.try_recv().ok()
    }
}

#[cfg(test)]
mod tests {
    use crate::daemon::link::Link;

    use super::*;

    #[test]
    fn the_channel_ends_carry_the_same_traffic() {
        let (link, peer) = Link::open();
        let (frames, _messages) = peer.into_channels();
        assert!(frames.send_blocking(DaemonFrame::Exit(1)).is_ok());
        drop(link);
        assert!(frames.send_blocking(DaemonFrame::Exit(2)).is_err());
    }

    #[test]
    fn a_dropped_view_ends_both_directions() {
        let (link, peer) = Link::open();
        drop(link);
        assert!(!peer.deliver_blocking(DaemonFrame::Exit(0)));
        assert_eq!(peer.next_message_blocking(), None);
        assert_eq!(peer.try_next_message(), None);
    }
}
