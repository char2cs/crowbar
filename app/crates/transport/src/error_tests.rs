use super::*;
use crate::response::{HOST_IDENTITY_CHANGED, HOST_UNREACHABLE, NO_CREDENTIAL, UNKNOWN_HOST};

#[test]
fn a_page_gets_a_marker_for_failures_it_can_act_on() {
    let host = HostId::new("host-1").unwrap();
    for (error, marker) in [
        (
            Error::IdentityChanged("x".into()),
            Some(HOST_IDENTITY_CHANGED),
        ),
        (Error::HostUnreachable("x".into()), Some(HOST_UNREACHABLE)),
        (Error::UpgradeTimeout, Some(HOST_UNREACHABLE)),
        (Error::NoCredential, Some(NO_CREDENTIAL)),
        (Error::UnknownHost(host), Some(UNKNOWN_HOST)),
        (Error::Forbidden("x".into()), None),
        (Error::TooLarge, None),
    ] {
        assert_eq!(error.marker(), marker, "{error}");
    }
}

#[test]
fn local_socket_errors_never_print_a_path() {
    let error = Error::LocalSocket(io::Error::new(io::ErrorKind::NotFound, "no such file"));
    assert!(!error.to_string().contains('/'), "{error}");
    assert!(
        std::error::Error::source(&error).is_some(),
        "the io error stays the source"
    );
}
