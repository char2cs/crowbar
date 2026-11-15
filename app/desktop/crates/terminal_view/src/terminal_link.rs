//! The view's side of the daemon connection.

use async_channel::{Receiver, Sender, unbounded};
use terminal::{ClientMessage, DaemonFrame};

use crate::terminal_peer::TerminalPeer;

/// What a [`crate::TerminalView`] reads frames from and writes messages to. The host connects the other
/// end ([`TerminalPeer`]) to the PTY websocket; input typed before the socket opens waits in the channel.
#[derive(Debug)]
pub struct TerminalLink {
    pub(crate) frames: Receiver<DaemonFrame>,
    pub(crate) outbound: Sender<ClientMessage>,
}

impl TerminalLink {
    /// A connected pair: the link for the view and the peer for the host.
    pub fn open() -> (Self, TerminalPeer) {
        let (frame_sender, frames) = unbounded();
        let (outbound, message_receiver) = unbounded();
        let link = Self { frames, outbound };
        (link, TerminalPeer::new(frame_sender, message_receiver))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn frames_flow_to_the_view_and_messages_to_the_host() {
        let (link, peer) = TerminalLink::open();
        assert!(peer.deliver_blocking(DaemonFrame::Exit(0)));
        assert_eq!(link.frames.try_recv().ok(), Some(DaemonFrame::Exit(0)));
        assert!(
            link.outbound
                .try_send(ClientMessage::Input("x".into()))
                .is_ok()
        );
        assert_eq!(
            peer.try_next_message(),
            Some(ClientMessage::Input("x".into()))
        );
    }
}
