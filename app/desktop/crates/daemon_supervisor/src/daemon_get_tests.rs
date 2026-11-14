use std::future::Future;
use std::io::ErrorKind;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio::io::{AsyncReadExt, AsyncWriteExt, DuplexStream};

use super::*;

/// What the fake daemon answers to the nth dial's request for a path: a status (0 hangs up
/// without answering) and a body.
type Answer = Arc<dyn Fn(usize, &str) -> (u16, String) + Send + Sync>;
type Refusal = Box<dyn Fn(usize) -> Option<std::io::Error> + Send + Sync>;

/// A daemon in memory: each dial gets an in-process connection that serves one HTTP exchange,
/// and a dial can be made to fail with any OS error. Dials are counted from 0.
pub(crate) struct FakeDaemon {
    answer: Answer,
    refuse: Refusal,
    dials: AtomicUsize,
}

impl FakeDaemon {
    pub(crate) fn answering(
        answer: impl Fn(usize, &str) -> (u16, String) + Send + Sync + 'static,
    ) -> Self {
        Self {
            answer: Arc::new(answer),
            refuse: Box::new(|_| None),
            dials: AtomicUsize::new(0),
        }
    }

    pub(crate) fn refusing(
        mut self,
        refuse: impl Fn(usize) -> Option<std::io::Error> + Send + Sync + 'static,
    ) -> Self {
        self.refuse = Box::new(refuse);
        self
    }

    pub(crate) fn dialled(&self) -> usize {
        self.dials.load(Ordering::SeqCst)
    }
}

async fn read_head(server: &mut DuplexStream) -> Vec<u8> {
    let mut head = Vec::new();
    let mut byte = [0_u8; 1];
    while !head.ends_with(b"\r\n\r\n") {
        if server.read(&mut byte).await.unwrap_or(0) == 0 {
            break;
        }
        head.extend_from_slice(&byte);
    }
    head
}

async fn serve(mut server: DuplexStream, index: usize, answer: Answer) {
    let head = read_head(&mut server).await;
    let text = String::from_utf8_lossy(&head);
    let target = text.split_whitespace().nth(1).unwrap_or_default();
    let (status, body) = answer(index, target);
    if status == 0 {
        return;
    }
    let reply = format!(
        "HTTP/1.1 {status} X\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
        body.len()
    );
    server.write_all(reply.as_bytes()).await.unwrap();
}

impl Dial for FakeDaemon {
    type Stream = DuplexStream;

    fn dial(&self) -> impl Future<Output = std::io::Result<DuplexStream>> + Send {
        let index = self.dials.fetch_add(1, Ordering::SeqCst);
        if let Some(error) = (self.refuse)(index) {
            return std::future::ready(Err(error));
        }
        let (client, server) = tokio::io::duplex(64 * 1024);
        tokio::spawn(serve(server, index, Arc::clone(&self.answer)));
        std::future::ready(Ok(client))
    }
}

#[tokio::test]
async fn a_successful_answer_is_returned_whole() {
    let daemon = FakeDaemon::answering(|_, target| (200, format!("echo {target}")));
    let body = get(&daemon, "/v0/health", 1024).await.unwrap();
    assert_eq!(body, b"echo /v0/health");
    assert_eq!(daemon.dialled(), 1, "one dial per request");
}

#[tokio::test]
async fn a_status_that_is_not_success_is_an_error_naming_it() {
    let daemon = FakeDaemon::answering(|_, _| (503, "{}".into()));
    let error = get(&daemon, "/v0/projects", 1024).await.unwrap_err();
    assert!(
        matches!(&error, Error::Status { status: 503, target } if target == "/v0/projects"),
        "{error:?}"
    );
}

#[tokio::test]
async fn an_answer_over_the_limit_is_refused_as_too_large_not_as_a_broken_connection() {
    let daemon = FakeDaemon::answering(|_, _| (200, "x".repeat(100)));
    let error = get(&daemon, "/big", 10).await.unwrap_err();
    assert!(
        matches!(error, Error::BodyTooLarge { limit: 10, .. }),
        "{error:?}"
    );
}

#[tokio::test]
async fn a_failed_dial_keeps_the_os_error_that_caused_it() {
    let daemon = FakeDaemon::answering(|_, _| (200, String::new()))
        .refusing(|_| Some(std::io::Error::from(ErrorKind::ConnectionRefused)));
    let error = get(&daemon, "/v0/health", 10).await.unwrap_err();
    assert!(
        matches!(&error, Error::Dial { source } if source.kind() == ErrorKind::ConnectionRefused),
        "{error:?}"
    );
}

#[tokio::test]
async fn a_daemon_that_hangs_up_without_answering_is_an_exchange_error() {
    let daemon = FakeDaemon::answering(|_, _| (0, String::new()));
    let error = get(&daemon, "/v0/health", 10).await.unwrap_err();
    assert!(matches!(error, Error::Exchange { .. }), "{error:?}");
    assert!(!error.is_descriptor_exhaustion(), "not a local failure");
}
