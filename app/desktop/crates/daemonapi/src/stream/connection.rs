use tokio::sync::mpsc;

#[derive(Debug)]
pub struct Connection {
    pub incoming: mpsc::Receiver<String>,
    pub outgoing: mpsc::UnboundedSender<String>,
}
