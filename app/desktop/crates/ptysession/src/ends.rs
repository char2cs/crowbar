//! The view's ends of a session.

use async_channel::{Receiver, Sender};
use terminal::{ClientMessage, DaemonFrame};

/// The view's ends of a session: where frames go and where its messages come from.
#[derive(Debug, Clone)]
pub struct Ends {
    /// Frames for the view.
    pub frames: Sender<DaemonFrame>,
    /// Messages from the view.
    pub messages: Receiver<ClientMessage>,
}

#[cfg(test)]
mod tests {
    use async_channel::unbounded;

    use super::*;

    #[test]
    fn the_ends_carry_frames_and_messages_independently() {
        let (frames, seen) = unbounded();
        let (sent, messages) = unbounded();
        let ends = Ends { frames, messages };
        ends.frames.try_send(DaemonFrame::Exit(0)).unwrap();
        sent.try_send(ClientMessage::Input("x".into())).unwrap();
        assert_eq!(seen.try_recv().unwrap(), DaemonFrame::Exit(0));
        assert_eq!(
            ends.messages.try_recv().unwrap(),
            ClientMessage::Input("x".into())
        );
    }
}
