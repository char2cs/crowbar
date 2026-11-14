use std::sync::Mutex;
use std::time::Duration;

use tokio::net::{TcpListener, TcpStream};
use tokio_util::sync::CancellationToken;

use super::*;
use crate::connection_book::ConnectionBook;
use crate::exact_origins::ExactOrigins;
use crate::host_registry::HostRegistry;
use crate::memory_token_store::MemoryTokenStore;
use crate::relay_config::RelayConfig;
use crate::token_book::TokenBook;
use crate::transport::Transport;

fn state(handshake_timeout: Duration) -> Arc<RelayState> {
    let transport = Transport::new(
        Arc::new(std::env::temp_dir().join("cb-session.sock")),
        Arc::new(HostRegistry::in_memory()),
        Arc::new(MemoryTokenStore::default()),
    );
    Arc::new(RelayState {
        transport: Arc::new(transport),
        config: RelayConfig {
            handshake_timeout,
            ..RelayConfig::default()
        },
        policy: Arc::new(ExactOrigins::new(vec![], "crowbar-ext://{extension}")),
        tokens: Mutex::new(TokenBook::new(Duration::from_secs(60))),
        connections: ConnectionBook::new(4, 4),
        port: 1,
        shutdown: CancellationToken::new(),
    })
}

#[tokio::test(start_paused = true)]
async fn a_client_that_never_finishes_the_handshake_is_dropped_at_the_deadline() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).await.unwrap();
    let silent = TcpStream::connect(listener.local_addr().unwrap())
        .await
        .unwrap();
    let (accepted, _) = listener.accept().await.unwrap();
    run(state(Duration::from_secs(10)), accepted).await;
    drop(silent);
}

#[tokio::test]
async fn garbage_instead_of_a_websocket_handshake_ends_the_session() {
    use tokio::io::AsyncWriteExt;
    let listener = TcpListener::bind(("127.0.0.1", 0)).await.unwrap();
    let mut client = TcpStream::connect(listener.local_addr().unwrap())
        .await
        .unwrap();
    let (accepted, _) = listener.accept().await.unwrap();
    client.write_all(b"GET / HTTP/1.1\r\n\r\n").await.unwrap();
    run(state(Duration::from_secs(10)), accepted).await;
}
