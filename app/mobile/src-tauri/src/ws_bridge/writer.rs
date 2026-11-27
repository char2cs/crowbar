use futures_util::stream::SplitSink;
use futures_util::SinkExt;
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::sync::{mpsc, oneshot};
use tokio_tungstenite::tungstenite::{Error as WsError, Message};
use tokio_tungstenite::WebSocketStream;

type Sink<T> = SplitSink<WebSocketStream<T>, Message>;

async fn send_all<T>(
    write: &mut Sink<T>,
    queue: &mut mpsc::UnboundedReceiver<Message>,
) -> Result<(), WsError>
where
    T: AsyncRead + AsyncWrite + Unpin,
{
    while let Some(message) = queue.recv().await {
        write.send(message).await?;
    }
    Ok(())
}

fn announce_dead(dead: oneshot::Sender<()>) {
    if dead.send(()).is_err() {
        log::debug!("ws bridge: writer died after its reader had finished");
    }
}

pub(super) async fn write_frames<T>(
    mut write: Sink<T>,
    mut queue: mpsc::UnboundedReceiver<Message>,
    dead: oneshot::Sender<()>,
) where
    T: AsyncRead + AsyncWrite + Unpin,
{
    if send_all(&mut write, &mut queue).await.is_err() {
        announce_dead(dead);
    }
    if let Err(error) = write.close().await {
        log::debug!("ws bridge: closing the write half failed: {error}");
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::bridge_fakes::ws_pair;
    use futures_util::StreamExt;

    #[tokio::test]
    async fn queued_messages_reach_the_peer_in_order_then_the_stream_closes() {
        queued_messages_reach_the_peer_in_order_run().await;
    }

    async fn queued_messages_reach_the_peer_in_order_run() {
        let (client, mut server) = ws_pair().await;
        let (write, _read) = client.split();
        let (tx, queue) = mpsc::unbounded_channel();
        let (dead, mut dead_rx) = oneshot::channel();
        let writer = tokio::spawn(write_frames(write, queue, dead));
        for text in ["one", "two"] {
            tx.send(Message::Text(text.into())).unwrap();
        }
        drop(tx);
        writer.await.unwrap();
        let mut seen = Vec::new();
        while let Some(Ok(Message::Text(text))) = server.next().await {
            seen.push(text.to_string());
        }
        assert_eq!(seen, ["one", "two"]);
        assert!(dead_rx.try_recv().is_err(), "a clean end is not a death");
    }

    #[tokio::test]
    async fn a_failed_write_announces_the_dead_writer() {
        let (client, server) = ws_pair().await;
        let (write, _read) = client.split();
        let (tx, queue) = mpsc::unbounded_channel();
        let (dead, dead_rx) = oneshot::channel();
        drop(server);
        let writer = tokio::spawn(write_frames(write, queue, dead));
        let queued = (0..64)
            .take_while(|_| tx.send(Message::Text("x".repeat(4096).into())).is_ok())
            .count();
        assert!(queued > 0, "the first message is always queued");
        writer.await.unwrap();
        assert!(dead_rx.await.is_ok(), "the reader must hear about it");
    }
}
