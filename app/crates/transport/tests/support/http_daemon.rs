use std::fmt::Write;
use std::sync::{Arc, Mutex};

use tokio::io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt};

/// One request as a fake daemon saw it.
#[derive(Debug, Clone)]
pub(crate) struct Seen {
    pub(crate) method: String,
    pub(crate) target: String,
    pub(crate) headers: Vec<(String, String)>,
    pub(crate) body: Vec<u8>,
}

impl Seen {
    pub(crate) fn header(&self, name: &str) -> Option<&str> {
        self.headers
            .iter()
            .find(|(key, _)| key.eq_ignore_ascii_case(name))
            .map(|(_, value)| value.as_str())
    }
}

pub(crate) type Log = Arc<Mutex<Vec<Seen>>>;
pub(crate) type Reply = (u16, Vec<(String, String)>, String);
pub(crate) type Handler = Arc<dyn Fn(&Seen) -> Reply + Send + Sync>;

async fn fill<S: AsyncRead + Unpin>(stream: &mut S, buffered: &mut Vec<u8>) -> Option<()> {
    let mut chunk = [0u8; 8192];
    let read = stream.read(&mut chunk).await.ok()?;
    if read == 0 {
        return None;
    }
    buffered.extend_from_slice(chunk.get(..read)?);
    Some(())
}

type Head = (String, String, Vec<(String, String)>);

fn parse_head(head: &str) -> Option<Head> {
    let mut lines = head.split("\r\n");
    let mut first = lines.next()?.split(' ');
    let (method, target) = (first.next()?.to_owned(), first.next()?.to_owned());
    let headers = lines
        .filter_map(|line| line.split_once(':'))
        .map(|(k, v)| (k.trim().to_owned(), v.trim().to_owned()))
        .collect();
    Some((method, target, headers))
}

async fn read_request<S: AsyncRead + Unpin>(
    stream: &mut S,
    buffered: &mut Vec<u8>,
) -> Option<Seen> {
    let end = loop {
        if let Some(at) = buffered.windows(4).position(|w| w == b"\r\n\r\n") {
            break at + 4;
        }
        fill(stream, buffered).await?;
    };
    let (method, target, headers) = parse_head(&String::from_utf8_lossy(buffered.get(..end)?))?;
    let length = headers
        .iter()
        .find(|(k, _)| k.eq_ignore_ascii_case("content-length"))
        .and_then(|(_, v)| v.parse::<usize>().ok())
        .unwrap_or(0);
    while buffered.len() < end + length {
        fill(stream, buffered).await?;
    }
    let body = buffered.get(end..end + length)?.to_vec();
    buffered.drain(..end + length);
    Some(Seen { method, target, headers, body })
}

/// Serves HTTP/1.1 with keep-alive on `stream` until it closes, recording what it saw.
pub(crate) async fn serve_http<S>(mut stream: S, handler: Handler, log: Log)
where
    S: AsyncRead + AsyncWrite + Unpin,
{
    let mut buffered = Vec::new();
    while let Some(seen) = read_request(&mut stream, &mut buffered).await {
        let (status, headers, body) = handler(&seen);
        log.lock().expect("log lock").push(seen);
        let mut reply = format!("HTTP/1.1 {status} X\r\ncontent-length: {}\r\n", body.len());
        for (name, value) in headers {
            write!(reply, "{name}: {value}\r\n").ok();
        }
        reply.push_str("\r\n");
        reply.push_str(&body);
        if stream.write_all(reply.as_bytes()).await.is_err() {
            return;
        }
    }
}
