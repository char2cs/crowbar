use tokio::sync::oneshot;

pub(crate) enum Message {
    Text(String),
    Flush(oneshot::Sender<()>),
}
