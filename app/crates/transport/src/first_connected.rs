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
    const SLOW: Duration = Duration::from_millis(100);

    #[derive(Clone, Copy)]
    enum Behaviour {
        Silent,
        Answers(Duration),
        Refuses,
    }

    struct Case {
        what: &'static str,
        addresses: &'static [Behaviour],
        expected: Result<u8, io::ErrorKind>,
        elapsed: Duration,
    }

    fn addr(last: u8) -> SocketAddr {
        SocketAddr::new(IpAddr::V4(Ipv4Addr::new(10, 0, 0, last)), 1)
    }

    fn last_octet(address: SocketAddr) -> u8 {
        match address {
            SocketAddr::V4(v4) => v4.ip().octets()[3],
            SocketAddr::V6(_) => 0,
        }
    }

    type Attempt = std::pin::Pin<Box<dyn Future<Output = io::Result<u8>> + Send>>;

    async fn attempt(
        last: u8,
        behaviour: Behaviour,
    ) -> io::Result<u8> {
        match behaviour {
            Behaviour::Silent => std::future::pending().await,
            Behaviour::Refuses => Err(io::Error::from(io::ErrorKind::ConnectionRefused)),
            Behaviour::Answers(after) => {
                tokio::time::sleep(after).await;
                Ok(last)
            },
        }
    }

    #[tokio::test(start_paused = true)]
    async fn the_first_address_to_connect_wins_and_a_dead_one_costs_at_most_a_stagger() {
        let cases = [
            Case {
                what: "a dead first address costs one stagger",
                addresses: &[Behaviour::Silent, Behaviour::Answers(Duration::ZERO)],
                expected: Ok(2),
                elapsed: STAGGER,
            },
            Case {
                what: "a slow first address that answers in time wins",
                addresses: &[Behaviour::Answers(SLOW), Behaviour::Answers(SLOW)],
                expected: Ok(1),
                elapsed: SLOW,
            },
            Case {
                what: "a refusal starts the next address at once",
                addresses: &[Behaviour::Refuses, Behaviour::Answers(Duration::ZERO)],
                expected: Ok(2),
                elapsed: Duration::ZERO,
            },
            Case {
                what: "every address refusing returns the last error",
                addresses: &[Behaviour::Refuses, Behaviour::Refuses],
                expected: Err(io::ErrorKind::ConnectionRefused),
                elapsed: Duration::ZERO,
            },
            Case {
                what: "no address is an error",
                addresses: &[],
                expected: Err(io::ErrorKind::Other),
                elapsed: Duration::ZERO,
            },
        ];
        for case in cases {
            let started = Instant::now();
            let behaviours = case.addresses;
            let count = u8::try_from(behaviours.len()).unwrap();
            let connect = move |address: SocketAddr| -> Attempt {
                let last = last_octet(address);
                let behaviour = behaviours.get(usize::from(last) - 1).copied().unwrap();
                Box::pin(attempt(last, behaviour))
            };
            let addresses = (1..=count).map(addr).collect();
            let outcome = first_connected(addresses, STAGGER, connect).await;
            let seen = outcome.map_err(|error| error.kind());
            assert_eq!(seen, case.expected, "{}", case.what);
            assert_eq!(started.elapsed(), case.elapsed, "{}", case.what);
        }
    }
}
