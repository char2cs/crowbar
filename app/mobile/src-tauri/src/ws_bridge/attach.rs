use futures_util::StreamExt;
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::sync::{mpsc, oneshot};
use tokio::task::JoinHandle;
use tokio_tungstenite::WebSocketStream;

use super::connection::Connection;
use super::frame_sink::FrameSink;
use super::open_request::OpenRequest;
use super::session::Session;
use super::writer::write_frames;
use super::ws_bridge_manager::WsBridgeManager;

pub(super) fn attach<T, S>(
    ws: WebSocketStream<T>,
    on_message: S,
    manager: &WsBridgeManager,
    request: OpenRequest,
) -> JoinHandle<()>
where
    T: AsyncRead + AsyncWrite + Unpin + Send + 'static,
    S: FrameSink,
{
    let (write, read) = ws.split();
    let (tx, queue) = mpsc::unbounded_channel();
    let (dead, writer_dead) = oneshot::channel();
    let writer = tokio::spawn(write_frames(write, queue, dead));
    let generation = manager.claim_generation();
    let (cancel, cancelled) = oneshot::channel();
    let connection = Connection {
        generation,
        window_label: request.window_label,
        host_id: request.host_id,
        tx,
        _cancel: cancel,
    };
    manager.register(request.conn_id.clone(), connection);
    let session = Session {
        read,
        on_message,
        cancelled,
        writer_dead,
        idle: request.read_idle_timeout,
        connections: manager.handle(),
        conn_id: request.conn_id,
        generation,
        writer,
    };
    tokio::spawn(session.serve())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::bridge_fakes::{frames, sentinel_sink, silent_sink, ws_pair};
    use crate::ws_bridge::frame::Frame;
    use crate::ws_bridge::ws_bridge_manager::locked;
    use futures_util::SinkExt;
    use std::time::Duration;
    use tokio_tungstenite::tungstenite::Message;

    fn request(idle: Option<Duration>) -> OpenRequest {
        OpenRequest {
            conn_id: "c1".to_owned(),
            path: "/v0/x".to_owned(),
            host_id: Some("h1".to_owned()),
            window_label: "main".to_owned(),
            read_idle_timeout: idle,
        }
    }

    fn is_empty(manager: &WsBridgeManager) -> bool {
        locked(&manager.connections).is_empty()
    }

    #[tokio::test]
    async fn a_daemon_close_retires_the_connection_and_tells_the_shim() {
        let (client, server) = ws_pair().await;
        let manager = WsBridgeManager::default();
        let (tx, closed) = oneshot::channel();
        let reader = attach(client, sentinel_sink(tx), &manager, request(None));
        drop(server);
        reader.await.expect("the reader ends");
        closed
            .await
            .expect("the shim must be told the daemon closed");
        assert!(is_empty(&manager), "a closed stream must leave the map");
    }

    #[tokio::test]
    async fn retiring_a_connection_by_any_route_ends_its_reader_against_a_silent_daemon() {
        retiring_a_connection_by_any_route_ends_run().await;
    }

    async fn retiring_a_connection_by_any_route_ends_run() {
        const CASES: &[&str] = &["close", "page-load", "host-removed"];
        for route in CASES {
            let (client, _server) = ws_pair().await;
            let manager = WsBridgeManager::default();
            let reader = attach(client, silent_sink(), &manager, request(None));
            match *route {
                "close" => manager.retire("c1"),
                "page-load" => manager.close_for_window("main"),
                _ => manager.close_for_host("h1"),
            }
            reader.await.expect(route);
            assert!(is_empty(&manager), "{route}");
        }
    }

    #[tokio::test]
    async fn text_and_binary_frames_are_forwarded_as_they_are() {
        let (client, mut server) = ws_pair().await;
        let manager = WsBridgeManager::default();
        let (sink, mut received) = frames();
        let reader = attach(client, sink, &manager, request(None));
        let payload = vec![0u8, b'h', 0xF0, 0x9F, 0x9A, 0x80];
        server.send(Message::Text("hi".into())).await.unwrap();
        server
            .send(Message::Binary(payload.clone().into()))
            .await
            .unwrap();
        assert_eq!(received.recv().await, Some(Frame::Text("hi".to_owned())));
        assert_eq!(received.recv().await, Some(Frame::Binary(payload)));
        manager.retire("c1");
        reader.await.unwrap();
    }

    #[tokio::test(start_paused = true)]
    async fn a_silent_half_open_socket_is_retired_and_announced() {
        let (client, _server) = ws_pair().await;
        let manager = WsBridgeManager::default();
        let (tx, closed) = oneshot::channel();
        let idle = Some(Duration::from_millis(50));
        let reader = attach(client, sentinel_sink(tx), &manager, request(idle));
        reader.await.expect("the idle timeout judges it dead");
        closed
            .await
            .expect("the frontend must be told to re-attach");
        assert!(is_empty(&manager), "a half-open connection must be retired");
    }
}
