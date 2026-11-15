//! A stand-in for `crowbar-api serve` used by the supervisor's tests. Its behaviour is chosen by
//! `FAKE_DAEMON_MODE`: `ok`, `migrating`, `steps` (reports no step, step 1 of 2, step 2 of 2,
//! then serves), `halted`, `crash` (exits at once), `die` (serves one health answer, then
//! exits), `noisy` (writes a megabyte that is not UTF-8 to stderr before serving), `wedge`
//! (accepts and never answers) or `stall` (answers the first request, then never answers).

use std::io::Write;
use std::process::ExitCode;

use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{UnixListener, UnixStream};

const OK: &str = r#"{"success":true,"data":{"status":"ok","pid":PID}}"#;
const MIGRATING: &str = r#"{"success":true,"data":{"status":"migrating"}}"#;
const STARTING: &str = r#"{"success":true,"data":{"status":"migrating","detail":null}}"#;
const STEP_ONE: &str = r#"{"success":true,"data":{"status":"migrating","detail":{"phase":"pre-open","step":{"number":1,"name":"first"},"index":1,"total":2}}}"#;
const STEP_TWO: &str = r#"{"success":true,"data":{"status":"migrating","detail":{"phase":"pre-open","step":{"number":2,"name":"second"},"index":2,"total":2}}}"#;
const HALTED: &str = r#"{"success":true,"data":{"status":"halted","detail":{"reason":"layout_newer","message":"layout too new"}}}"#;
const NOISE_BYTES: usize = 1 << 20;

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
        ("steps", 1) => STARTING.to_owned(),
        ("steps", 2) => STEP_ONE.to_owned(),
        ("steps", 3) => STEP_TWO.to_owned(),
        _ => OK.replace("PID", &std::process::id().to_string()),
    }
}

async fn answer(
    mut stream: UnixStream,
    mode: &str,
    served: u32,
) {
    let mut buf = vec![0u8; 4096];
    let Ok(read) = stream.read(&mut buf).await else {
        return;
    };
    let head = String::from_utf8_lossy(buf.get(..read).unwrap_or_default()).into_owned();
    let body = if head.starts_with("GET /v0/health ") {
        health_body(mode, served)
    } else {
        "{}".to_owned()
    };
    let reply = format!(
        "HTTP/1.1 200 OK\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
        body.len()
    );
    stream.write_all(reply.as_bytes()).await.ok();
}

async fn hold(stream: UnixStream) {
    let _held = stream;
    std::future::pending::<()>().await;
}

async fn serve(
    listener: UnixListener,
    mode: String,
) -> ExitCode {
    let mut served = 0u32;
    loop {
        let Ok((stream, _)) = listener.accept().await else {
            return ExitCode::FAILURE;
        };
        served += 1;
        if mode == "wedge" || (mode == "stall" && served > 1) {
            tokio::spawn(hold(stream));
            continue;
        }
        answer(stream, &mode, served).await;
        if mode == "die" {
            return ExitCode::from(3);
        }
    }
}

#[tokio::main(flavor = "current_thread")]
async fn main() -> ExitCode {
    let mode = std::env::var("FAKE_DAEMON_MODE").unwrap_or_else(|_| "ok".to_owned());
    if mode == "crash" {
        return ExitCode::from(2);
    }
    let Some(path) = socket_arg() else {
        return ExitCode::from(64);
    };
    if mode == "noisy"
        && std::io::stderr()
            .write_all(&vec![0xff; NOISE_BYTES])
            .is_err()
    {
        return ExitCode::from(66);
    }
    std::fs::remove_file(&path).ok();
    match UnixListener::bind(&path) {
        Ok(listener) => serve(listener, mode).await,
        Err(_) => ExitCode::from(65),
    }
}
