use tokio::sync::oneshot;

pub(crate) enum LogMessage {
    Text(String),
    Flush(oneshot::Sender<()>),
}
