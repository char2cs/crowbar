use std::path::PathBuf;

use tokio::net::UnixStream;

use crate::dial::Dial;

pub(crate) struct SocketDial {
    path: PathBuf,
}

impl SocketDial {
    pub(crate) fn new(path: impl Into<PathBuf>) -> Self {
        Self { path: path.into() }
    }
}

impl Dial for SocketDial {
    type Stream = UnixStream;

    async fn dial(&self) -> std::io::Result<UnixStream> {
        UnixStream::connect(&self.path).await
    }
}

#[cfg(test)]
mod tests {
    use std::io::ErrorKind;

    use super::*;

    #[tokio::test]
    async fn dialling_a_socket_that_does_not_exist_reports_the_os_error() {
        let error = SocketDial::new(PathBuf::from("/nonexistent/absent.sock"))
            .dial()
            .await
            .unwrap_err();
        assert_eq!(error.kind(), ErrorKind::NotFound);
    }
}
