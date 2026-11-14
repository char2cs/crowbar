use tokio::io::{AsyncReadExt, AsyncWriteExt};

use super::*;
use crate::daemon_get::tests::FakeDaemon;

#[tokio::test]
async fn every_dial_is_a_new_independent_connection() {
    let daemon = FakeDaemon::answering(|index, _| (200, format!("answer {index}")));
    let (mut first, mut second) = (daemon.dial().await.unwrap(), daemon.dial().await.unwrap());
    let request = b"GET /v0/health HTTP/1.1\r\nhost: x\r\n\r\n";
    second.write_all(request).await.unwrap();
    first.write_all(request).await.unwrap();
    let (mut a, mut b) = (String::new(), String::new());
    first.read_to_string(&mut a).await.unwrap();
    second.read_to_string(&mut b).await.unwrap();
    assert!(a.ends_with("answer 0"), "{a}");
    assert!(b.ends_with("answer 1"), "{b}");
    assert_eq!(daemon.dialled(), 2);
}
