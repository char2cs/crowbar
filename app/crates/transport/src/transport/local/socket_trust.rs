use std::path::Path;

use rustix::process::geteuid;
use tokio::net::UnixStream;

use crate::error::Error;
use crate::transport::local::adapters::file_owner;

fn untrusted(why: String) -> Error {
    Error::UntrustedSocket(why)
}

pub(crate) fn require_same_owner(
    what: &str,
    found: u32,
    own: u32,
) -> Result<(), Error> {
    if found == own {
        return Ok(());
    }
    Err(untrusted(format!(
        "{what} belongs to user {found}, not {own}"
    )))
}

pub(crate) fn verify_file(
    path: &Path,
    own: u32,
) -> Result<(), Error> {
    let owner = file_owner(path).map_err(Error::LocalSocket)?;
    require_same_owner("the socket file", owner, own)
}

pub(crate) fn verify_peer(
    stream: &UnixStream,
    own: u32,
) -> Result<(), Error> {
    let peer = stream.peer_cred().map_err(Error::LocalSocket)?;
    require_same_owner("the process behind the socket", peer.uid(), own)
}

pub(crate) fn verify(
    path: &Path,
    stream: &UnixStream,
) -> Result<(), Error> {
    let own = geteuid().as_raw();
    verify_file(path, own)?;
    verify_peer(stream, own)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_our_own_user_is_trusted() {
        assert!(require_same_owner("x", 501, 501).is_ok());
        let cases = [0, 502, u32::MAX];
        for other in cases {
            let err = require_same_owner("x", other, 501).unwrap_err();
            assert!(matches!(err, Error::UntrustedSocket(_)), "{other}");
        }
    }

    #[tokio::test]
    #[ignore = "real-infra: binds a real unix socket to read its owner uid"]
    async fn a_socket_made_by_this_user_passes_and_one_claimed_by_another_does_not() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("d.sock");
        let listener = tokio::net::UnixListener::bind(&path).unwrap();
        let client = UnixStream::connect(&path).await.unwrap();
        let _server = listener.accept().await.unwrap();
        assert!(verify(&path, &client).is_ok());
        let own = geteuid().as_raw();
        assert!(verify_file(&path, own.wrapping_add(1)).is_err());
        assert!(verify_peer(&client, own.wrapping_add(1)).is_err());
    }

    #[test]
    fn a_missing_socket_file_is_a_connect_error_not_trust() {
        let err = verify_file(Path::new("/nonexistent/crowbar.sock"), 0).unwrap_err();
        assert!(matches!(err, Error::LocalSocket(_)));
    }
}
