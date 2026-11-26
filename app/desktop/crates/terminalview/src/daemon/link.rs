//! The view's side of the daemon connection.

use async_channel::{Receiver, Sender, bounded};
use terminal::{ClientMessage, DaemonFrame};

use crate::daemon::peer::Peer;

const FRAME_BACKLOG: usize = 64;
const MESSAGE_BACKLOG: usize = 1024;

/// What a [`crate::TerminalView`] reads frames from and writes messages to. The host connects the other
/// end ([`Peer`]) to the PTY websocket; input typed before the socket opens waits in the channel. Both channels are bounded: a host
/// that awaits `deliver` stalls its socket read while the view catches up, which backpressures the PTY.
#[derive(Debug)]
pub struct Link {
    pub(crate) frames: Receiver<DaemonFrame>,
    pub(crate) outbound: Sender<ClientMessage>,
}

impl Link {
    /// A connected pair: the link for the view and the peer for the host.
    pub fn open() -> (Self, Peer) {
        let (frame_sender, frames) = bounded(FRAME_BACKLOG);
        let (outbound, message_receiver) = bounded(MESSAGE_BACKLOG);
        let link = Self { frames, outbound };
        (link, Peer::new(frame_sender, message_receiver))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn frames_flow_to_the_view_and_messages_to_the_host_up_to_a_bound() {
        let (link, peer) = Link::open();
        let (frames, messages) = peer.into_channels();
        assert!(frames.try_send(DaemonFrame::Exit(0)).is_ok());
        assert_eq!(link.frames.try_recv().ok(), Some(DaemonFrame::Exit(0)));
        let input = ClientMessage::Input("x".into());
        assert!(link.outbound.try_send(input.clone()).is_ok());
        assert_eq!(messages.try_recv().ok(), Some(input));
        for _ in 0..FRAME_BACKLOG {
            assert!(frames.try_send(DaemonFrame::Exit(0)).is_ok());
        }
        let stalled = frames.try_send(DaemonFrame::Exit(0));
        assert!(
            stalled.is_err_and(|error| error.is_full()),
            "the host is stalled"
        );
    }
}
