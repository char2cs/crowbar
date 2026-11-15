use std::io;

use tokio::io::{DuplexStream, duplex};
use tokio::sync::mpsc;
use transport::Acceptor;

const PIPE_BYTES: usize = 1024 * 1024;
const BACKLOG: usize = 16;

/// The relay's side of an in-memory network: every dial arrives as one end of a pipe.
pub(crate) struct MemoryAcceptor {
    port: u16,
    incoming: mpsc::Receiver<DuplexStream>,
}

/// The pane's side: dials the relay through a pipe, so no test opens a socket.
#[derive(Clone)]
pub(crate) struct MemoryDialer {
    port: u16,
    outgoing: mpsc::Sender<DuplexStream>,
}

pub(crate) fn memory_listener(port: u16) -> (MemoryAcceptor, MemoryDialer) {
    let (outgoing, incoming) = mpsc::channel(BACKLOG);
    (
        MemoryAcceptor { port, incoming },
        MemoryDialer { port, outgoing },
    )
}

impl Acceptor for MemoryAcceptor {
    type Stream = DuplexStream;

    fn local_port(&self) -> io::Result<u16> {
        Ok(self.port)
    }

    async fn accept(&mut self) -> io::Result<DuplexStream> {
        self.incoming
            .recv()
            .await
            .ok_or_else(|| io::Error::from(io::ErrorKind::BrokenPipe))
    }
}

impl MemoryDialer {
    /// The port the relay claims to listen on.
    pub(crate) fn port(&self) -> u16 {
        self.port
    }

    /// Opens a connection, refused once the relay stopped accepting.
    pub(crate) async fn connect(&self) -> io::Result<DuplexStream> {
        let (client, server) = duplex(PIPE_BYTES);
        self.outgoing
            .send(server)
            .await
            .map_err(|_| io::Error::from(io::ErrorKind::ConnectionRefused))?;
        Ok(client)
    }
}
