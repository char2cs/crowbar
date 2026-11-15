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
        match settled_or_staggered(&mut running, stagger, queue.as_slice()).await {
            Some(Ok(connected)) => return Ok(connected),
            Some(Err(e)) => last_error = Some(e),
            None => {},
        }
        start_next(&mut queue, &mut running, &connect);
    }
    Err(last_error.unwrap_or_else(|| io::Error::other("no address to connect to")))
}

async fn settled_or_staggered<T: 'static>(
    running: &mut JoinSet<io::Result<T>>,
    stagger: Duration,
    waiting: &[SocketAddr],
) -> Option<io::Result<T>> {
    tokio::select! {
        Some(joined) = running.join_next() => Some(settled(joined)),
        () = tokio::time::sleep(stagger), if !waiting.is_empty() => None,
    }
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
mod tests {
    use std::net::{IpAddr, Ipv4Addr};

    use tokio::time::Instant;

    use super::*;

    const STAGGER: Duration = Duration::from_millis(250);

    fn addr(last: u8) -> SocketAddr {
        SocketAddr::new(IpAddr::V4(Ipv4Addr::new(10, 0, 0, last)), 1)
    }

    type Attempt = std::pin::Pin<Box<dyn Future<Output = io::Result<u8>> + Send>>;

    async fn reply(
        last: u8,
        after: Option<Duration>,
    ) -> io::Result<u8> {
        let Some(after) = after else {
            return std::future::pending().await;
        };
        tokio::time::sleep(after).await;
        Ok(last)
    }

    fn answer(
        slow: &'static [u8],
        after: Duration,
    ) -> impl Fn(SocketAddr) -> Attempt {
        move |address| {
            let last = match address {
                SocketAddr::V4(v4) => v4.ip().octets()[3],
                SocketAddr::V6(_) => 0,
            };
            let after = (!slow.contains(&last)).then_some(after);
            Box::pin(reply(last, after))
        }
    }

    #[tokio::test(start_paused = true)]
    async fn a_dead_first_address_costs_one_stagger_not_the_whole_budget() {
        let started = Instant::now();
        let connected = first_connected(
            vec![addr(1), addr(2)],
            STAGGER,
            answer(&[1], Duration::ZERO),
        )
        .await
        .unwrap();
        assert_eq!(connected, 2);
        assert_eq!(started.elapsed(), STAGGER);
    }

    fn refuse(address: SocketAddr) -> io::Result<u8> {
        if address == addr(1) {
            return Err(io::Error::from(io::ErrorKind::ConnectionRefused));
        }
        Ok(2)
    }

    #[tokio::test(start_paused = true)]
    async fn a_failed_attempt_starts_the_next_address_at_once() {
        let started = Instant::now();
        let refuse_first = |address: SocketAddr| std::future::ready(refuse(address));
        assert_eq!(
            first_connected(vec![addr(1), addr(2)], STAGGER, refuse_first)
                .await
                .unwrap(),
            2
        );
        assert_eq!(started.elapsed(), Duration::ZERO);
    }

    #[tokio::test(start_paused = true)]
    async fn a_slow_first_address_that_answers_in_time_wins() {
        let connected = first_connected(
            vec![addr(1), addr(2)],
            STAGGER,
            answer(&[], Duration::from_millis(100)),
        )
        .await
        .unwrap();
        assert_eq!(connected, 1);
    }

    async fn refuse_all(_: SocketAddr) -> io::Result<u8> {
        Err(io::Error::from(io::ErrorKind::ConnectionRefused))
    }

    #[tokio::test(start_paused = true)]
    async fn when_every_address_fails_the_last_error_is_returned() {
        let error = first_connected(vec![addr(1), addr(2)], STAGGER, refuse_all)
            .await
            .unwrap_err();
        assert_eq!(error.kind(), io::ErrorKind::ConnectionRefused);
        let none = first_connected(Vec::new(), STAGGER, refuse_all)
            .await
            .unwrap_err();
        assert_eq!(none.kind(), io::ErrorKind::Other);
    }
}
