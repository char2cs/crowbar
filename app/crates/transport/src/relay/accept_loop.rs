use std::io;
use std::sync::Arc;
use std::time::Duration;

use tokio::io::{AsyncRead, AsyncWrite, AsyncWriteExt};
use tokio::net::{TcpListener, TcpStream};
use tokio::sync::Semaphore;
use tokio::time::error::Elapsed;

use crate::relay::acceptor::Acceptor;
use crate::relay::session;
use crate::relay::state::State;

const ACCEPT_BACKOFF: Duration = Duration::from_millis(50);

pub(crate) async fn accept_loop<A: Acceptor>(
    mut acceptor: A,
    state: Arc<State>,
) {
    let handshakes = Arc::new(Semaphore::new(state.config.max_handshakes));
    while let Some(accepted) = state.shutdown.run_until_cancelled(acceptor.accept()).await {
        greet(&state, &handshakes, accepted).await;
    }
}

impl Acceptor for TcpListener {
    type Stream = TcpStream;

    fn local_port(&self) -> io::Result<u16> {
        Ok(self.local_addr()?.port())
    }

    async fn accept(&mut self) -> io::Result<TcpStream> {
        let (stream, _peer) = TcpListener::accept(self).await?;
        Ok(stream)
    }
}

async fn greet<S>(
    state: &Arc<State>,
    handshakes: &Arc<Semaphore>,
    accepted: io::Result<S>,
) where
    S: AsyncRead + AsyncWrite + Unpin + Send + 'static,
{
    let Ok(stream) = accepted else {
        return tokio::time::sleep(ACCEPT_BACKOFF).await;
    };
    let Ok(permit) = Arc::clone(handshakes).try_acquire_owned() else {
        return turn_away(stream, state.config.close_grace);
    };
    tokio::spawn(session::run(Arc::clone(state), stream, permit));
}

fn turn_away<S>(
    stream: S,
    grace: Duration,
) where
    S: AsyncWrite + Unpin + Send + 'static,
{
    tokio::spawn(refuse_busy(stream, grace));
}

async fn refuse_busy<S>(
    mut stream: S,
    grace: Duration,
) where
    S: AsyncWrite + Unpin,
{
    const BUSY: &[u8] =
        b"HTTP/1.1 503 Service Unavailable\r\nConnection: close\r\nContent-Length: 0\r\n\r\n";
    match tokio::time::timeout(grace, stream.write_all(BUSY)).await {
        Ok(Ok(())) => {},
        Ok(Err(e)) => refusal_not_delivered(&e),
        Err(elapsed) => refusal_not_taken(&elapsed),
    }
}

fn refusal_not_delivered(e: &io::Error) {
    tracing::debug!("relay is busy and the refusal was not delivered: {e}");
}

fn refusal_not_taken(elapsed: &Elapsed) {
    tracing::debug!("relay is busy and the refusal was not taken in time: {elapsed}");
}

#[cfg(test)]
mod tests {
    use tokio::io::{AsyncReadExt, duplex};

    use super::*;

    #[tokio::test]
    async fn a_refused_connection_is_told_the_relay_is_busy_and_closed() {
        let (client, server) = duplex(1024);
        refuse_busy(server, Duration::from_secs(1)).await;
        let mut answer = String::new();
        let mut client = client;
        client.read_to_string(&mut answer).await.unwrap();
        assert!(
            answer.starts_with("HTTP/1.1 503 Service Unavailable"),
            "{answer}"
        );
        assert!(answer.contains("Connection: close"), "{answer}");
    }

    #[tokio::test]
    async fn a_refusal_nobody_reads_is_abandoned_after_the_grace_period() {
        let (_client, server) = duplex(1);
        tokio::time::pause();
        refuse_busy(server, Duration::from_secs(1)).await;
    }
}
