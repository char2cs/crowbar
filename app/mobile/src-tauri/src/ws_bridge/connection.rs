use tokio::sync::{mpsc, oneshot};
use tokio_tungstenite::tungstenite::Message;

pub(super) struct Connection {
    pub(super) generation: u64,
    pub(super) window_label: String,
    pub(super) host_id: Option<String>,
    pub(super) tx: mpsc::UnboundedSender<Message>,
    pub(super) _cancel: oneshot::Sender<()>,
}
