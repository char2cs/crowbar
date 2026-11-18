//! The host's side of the daemon connection.

use async_channel::{Receiver, Sender};
use terminal::{ClientMessage, DaemonFrame};

/// Where the host puts frames read from the PTY websocket and takes messages to write to it.
#[derive(Debug, Clone)]
pub struct TerminalPeer {
    frames: Sender<DaemonFrame>,
    messages: Receiver<ClientMessage>,
}

impl TerminalPeer {
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

    /// Hands a frame from the daemon to the view; false once the view is gone.
    pub async fn deliver(
        &self,
        frame: DaemonFrame,
    ) -> bool {
        self.frames.send(frame).await.is_ok()
    }

    /// The next message the view wants sent to the daemon; `None` once the view is gone.
    pub async fn next_message(&self) -> Option<ClientMessage> {
        self.messages.recv().await.ok()
    }

    /// Like [`Self::deliver`] for a host that reads the socket on its own thread.
    pub fn deliver_blocking(
        &self,
        frame: DaemonFrame,
    ) -> bool {
        self.frames.send_blocking(frame).is_ok()
    }

    /// Like [`Self::next_message`] for a host that writes the socket on its own thread.
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
    use crate::terminal_link::TerminalLink;

    use super::*;

    #[test]
    fn the_channel_ends_carry_the_same_traffic() {
        let (link, peer) = TerminalLink::open();
        let (frames, _messages) = peer.into_channels();
        assert!(frames.send_blocking(DaemonFrame::Exit(1)).is_ok());
        drop(link);
        assert!(frames.send_blocking(DaemonFrame::Exit(2)).is_err());
    }

    #[test]
    fn a_dropped_view_ends_both_directions() {
        let (link, peer) = TerminalLink::open();
        drop(link);
        assert!(!peer.deliver_blocking(DaemonFrame::Exit(0)));
        assert_eq!(peer.next_message_blocking(), None);
        assert_eq!(peer.try_next_message(), None);
    }
}
