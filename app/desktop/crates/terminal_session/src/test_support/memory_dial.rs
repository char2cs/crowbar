use std::io;
use std::sync::Arc;

use tokio::io::{DuplexStream, duplex};
use tokio::sync::mpsc::{UnboundedReceiver, UnboundedSender, unbounded_channel};

use crate::dial::{Dial, Dialing, Link};

const BUFFER: usize = 64 * 1024;

/// A dialer whose every connection is an in-memory pipe, and the server ends of those pipes.
pub(crate) fn memory_dial() -> (Dial, UnboundedReceiver<DuplexStream>) {
    let (servers, accepted) = unbounded_channel();
    let dial: Dial = Arc::new(move |_address| connect(&servers));
    (dial, accepted)
}

fn connect(servers: &UnboundedSender<DuplexStream>) -> Dialing {
    let (client, server) = duplex(BUFFER);
    let sent = servers.send(server);
    Box::pin(async move {
        sent.map_err(|_| io::Error::from(io::ErrorKind::ConnectionRefused))?;
        Ok(Box::new(client) as Link)
    })
}

/// A dialer that cannot reach anything.
pub(crate) fn refusing_dial() -> Dial {
    Arc::new(|_address| Box::pin(async { Err(io::Error::from(io::ErrorKind::ConnectionRefused)) }))
}
