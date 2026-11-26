//! The `late` mode: health is unserved until the test creates `<socket>.serve`, and each unserved
//! answer is announced by dialling `<socket>.spent`; that it listens, by dialling `<socket>.up`.

use std::os::unix::net::UnixStream;
use std::path::Path;

const LATE: &str = "late";
const HEALTH: &str = "GET /v0/health ";
const UNSERVED: &str =
    "HTTP/1.1 503 Service Unavailable\r\ncontent-length: 2\r\nconnection: close\r\n\r\n{}";

pub(crate) fn unserved_reply(
    head: &str,
    mode: &str,
    socket: Option<&str>,
) -> Option<String> {
    let socket = socket?;
    let gated = !Path::new(&format!("{socket}.serve")).exists();
    (mode == LATE && head.starts_with(HEALTH) && gated).then(|| UNSERVED.to_owned())
}

pub(crate) fn tell_spent(socket: Option<&str>) {
    let Some(socket) = socket else {
        return;
    };
    if let Err(error) = UnixStream::connect(format!("{socket}.spent")) {
        tracing::debug!("no test listens for unserved answers: {error}");
    }
}

pub(crate) fn tell_up(
    mode: &str,
    socket: &str,
) {
    if mode != LATE {
        return;
    }
    if let Err(error) = UnixStream::connect(format!("{socket}.up")) {
        tracing::debug!("no test listens for the socket: {error}");
    }
}
