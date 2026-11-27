use async_channel::{Receiver, Sender};
use terminal::{ClientMessage, DaemonFrame};

#[derive(Debug, Clone)]
pub struct Ends {
    pub frames: Sender<DaemonFrame>,
    pub messages: Receiver<ClientMessage>,
}
