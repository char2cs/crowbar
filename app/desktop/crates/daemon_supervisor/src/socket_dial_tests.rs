use std::io::ErrorKind;

use super::*;

#[tokio::test]
async fn dialling_a_socket_that_does_not_exist_reports_the_os_error() {
    let dir = tempfile::tempdir().unwrap();
    let error = SocketDial::new(dir.path().join("absent.sock"))
        .dial()
        .await
        .unwrap_err();
    assert_eq!(error.kind(), ErrorKind::NotFound);
}
