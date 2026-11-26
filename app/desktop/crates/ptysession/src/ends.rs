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
