use std::future::Future;
use std::io;
use std::net::SocketAddr;
use std::time::Duration;

use tokio::task::{JoinError, JoinSet};

pub(crate) async fn first_connected<T, C, F>(
    addrs: Vec<SocketAddr>,
    stagger: Duration,
    connect: C,
) -> io::Result<T>
where
    T: Send + 'static,
    C: Fn(SocketAddr) -> F,
    F: Future<Output = io::Result<T>> + Send + 'static,
{
    let mut queue = addrs.into_iter();
    let mut running = JoinSet::new();
    let mut last_error = None;
    start_next(&mut queue, &mut running, &connect);
    while !running.is_empty() {
        tokio::select! {
            Some(joined) = running.join_next() => match settled(joined) {
                Ok(connected) => return Ok(connected),
                Err(e) => last_error = Some(e),
            },
            () = tokio::time::sleep(stagger), if !queue.as_slice().is_empty() => {},
        }
        start_next(&mut queue, &mut running, &connect);
    }
    Err(last_error.unwrap_or_else(|| io::Error::other("no address to connect to")))
}

fn start_next<T, C, F>(
    queue: &mut std::vec::IntoIter<SocketAddr>,
    running: &mut JoinSet<io::Result<T>>,
    connect: &C,
) where
    T: Send + 'static,
    C: Fn(SocketAddr) -> F,
    F: Future<Output = io::Result<T>> + Send + 'static,
{
    if let Some(addr) = queue.next() {
        running.spawn(connect(addr));
    }
}

fn settled<T>(joined: Result<io::Result<T>, JoinError>) -> io::Result<T> {
    joined.unwrap_or_else(|e| Err(io::Error::other(e)))
}

#[cfg(test)]
#[path = "first_connected_tests.rs"]
mod tests;
