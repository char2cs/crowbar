//! A stand-in for `crowbar-api serve` used by the supervisor's tests. Its behaviour is chosen by
//! `FAKE_DAEMON_MODE`: `ok`, `migrating`, `steps` (reports no step, step 1 of 2, step 2 of 2,
//! then serves), `halted`, `crash` (exits at once), `die` (serves one health answer, then
//! exits), `noisy` (writes a megabyte that is not UTF-8 to stderr before serving), `wedge`
//! (accepts and never answers), `stall` (answers the first request, then never answers),
//! `deaf` (serves, and ignores SIGTERM) or `late` (answers health with 503 until `<socket>.serve`
//! exists, dialling `<socket>.spent` after each such answer so a test hears it).

mod late;

use std::io::Write;
use std::process::ExitCode;

use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{UnixListener, UnixStream};
use tokio::signal::unix::{Signal, SignalKind, signal};

const OK: &str = r#"{"success":true,"data":{"status":"ok","pid":PID}}"#;
const MIGRATING: &str = r#"{"success":true,"data":{"status":"migrating"}}"#;
const STARTING: &str = r#"{"success":true,"data":{"status":"migrating","detail":null}}"#;
const STEP_ONE: &str = r#"{"success":true,"data":{"status":"migrating","detail":{"phase":"pre-open","step":{"number":1,"name":"first"},"index":1,"total":2}}}"#;
const STEP_TWO: &str = r#"{"success":true,"data":{"status":"migrating","detail":{"phase":"pre-open","step":{"number":2,"name":"second"},"index":2,"total":2}}}"#;
const HALTED: &str = r#"{"success":true,"data":{"status":"halted","detail":{"reason":"layout_newer","message":"layout too new"}}}"#;
const STEPS: &str = "steps";
const NOISE_BYTES: usize = 1 << 20;
const NOISE_BYTE: u8 = 0xff;
const READ_BUFFER_BYTES: usize = 4096;
const THIRD_REQUEST: u32 = 3;
const EXIT_DIED: u8 = 3;
const EXIT_USAGE: u8 = 64;
const EXIT_BIND: u8 = 65;
const EXIT_NOISE: u8 = 66;
const EXIT_SIGNAL: u8 = 67;

fn socket_arg() -> Option<String> {
    let args: Vec<String> = std::env::args().collect();
    let host = args
        .iter()
        .position(|a| a == "--host")
        .and_then(|at| args.get(at + 1))?;
    host.strip_prefix("unix://").map(str::to_owned)
}

fn health_body(
    mode: &str,
    served: u32,
) -> String {
    match (mode, served) {
        ("migrating", _) => MIGRATING.to_owned(),
        ("halted", _) => HALTED.to_owned(),
        (STEPS, 1) => STARTING.to_owned(),
        (STEPS, 2) => STEP_ONE.to_owned(),
        (STEPS, THIRD_REQUEST) => STEP_TWO.to_owned(),
        _ => OK.replace("PID", &std::process::id().to_string()),
    }
}

fn reply_body(
    head: &str,
    mode: &str,
    served: u32,
) -> String {
    let body = Some(head)
        .filter(|head| head.starts_with("GET /v0/health "))
        .map_or_else(|| "{}".to_owned(), |_| health_body(mode, served));
    format!(
        "HTTP/1.1 200 OK\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
        body.len()
    )
}

async fn answer(
    mut stream: UnixStream,
    mode: &str,
    served: u32,
) {
    let mut buf = vec![0u8; READ_BUFFER_BYTES];
    let Ok(read) = stream.read(&mut buf).await else {
        return;
    };
    let head = String::from_utf8_lossy(buf.get(..read).unwrap_or_default()).into_owned();
    let socket = socket_arg();
    let unserved = late::unserved_reply(&head, mode, socket.as_deref());
    let late = unserved.is_some();
    let reply = unserved.unwrap_or_else(|| reply_body(&head, mode, served));
    if let Err(error) = stream.write_all(reply.as_bytes()).await {
        client_left(&error);
    }
    if late {
        late::tell_spent(socket.as_deref());
    }
}

fn client_left(error: &std::io::Error) {
    tracing::debug!("the client left before its answer: {error}");
}

async fn hold(stream: UnixStream) {
    let _held = stream;
    std::future::pending::<()>().await;
}

fn holds_connection(
    mode: &str,
    served: u32,
) -> bool {
    let stalls_now = mode == "stall" && served > 1;
    mode == "wedge" || stalls_now
}

async fn serve_one(
    listener: &UnixListener,
    mode: &str,
    served: u32,
) -> Option<ExitCode> {
    let Ok((stream, _)) = listener.accept().await else {
        return Some(ExitCode::FAILURE);
    };
    if holds_connection(mode, served) {
        tokio::spawn(hold(stream));
        return None;
    }
    answer(stream, mode, served).await;
    (mode == "die").then_some(ExitCode::from(EXIT_DIED))
}

async fn serve(
    listener: UnixListener,
    mode: String,
) -> ExitCode {
    let mut served = 0u32;
    let mut finished = None;
    while finished.is_none() {
        served += 1;
        finished = serve_one(&listener, &mode, served).await;
    }
    finished.unwrap_or(ExitCode::FAILURE)
}

fn noise_failed(mode: &str) -> bool {
    mode == "noisy"
        && std::io::stderr()
            .write_all(&vec![NOISE_BYTE; NOISE_BYTES])
            .is_err()
}

fn clear_stale_socket(path: &str) -> Result<(), rustix::io::Errno> {
    match rustix::fs::unlink(path) {
        Err(rustix::io::Errno::NOENT) => Ok(()),
        other => other,
    }
}

fn deaf_to_term(mode: &str) -> std::io::Result<Option<Signal>> {
    if mode != "deaf" {
        return Ok(None);
    }
    signal(SignalKind::terminate()).map(Some)
}

#[tokio::main(flavor = "current_thread")]
async fn main() -> ExitCode {
    let mode = std::env::var("FAKE_DAEMON_MODE").unwrap_or_else(|_| "ok".to_owned());
    if mode == "crash" {
        return ExitCode::from(2);
    }
    let Some(path) = socket_arg() else {
        return ExitCode::from(EXIT_USAGE);
    };
    let Ok(_deaf) = deaf_to_term(&mode) else {
        return ExitCode::from(EXIT_SIGNAL);
    };
    if noise_failed(&mode) {
        return ExitCode::from(EXIT_NOISE);
    }
    if let Err(error) = clear_stale_socket(&path) {
        tracing::debug!("stale socket {path}: {error}");
        return ExitCode::from(EXIT_BIND);
    }
    let Ok(listener) = UnixListener::bind(&path) else {
        return ExitCode::from(EXIT_BIND);
    };
    serve(listener, mode).await
}
