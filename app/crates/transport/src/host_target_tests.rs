use super::*;

fn id(text: &str) -> HostId {
    HostId::new(text).unwrap()
}

#[test]
fn local_and_localhost_name_the_local_daemon() {
    assert_eq!(HostTarget::of(&id("local")), HostTarget::Local);
    assert_eq!(HostTarget::of(&id("localhost")), HostTarget::Local);
    assert_eq!(
        HostTarget::of(&id("host-1")),
        HostTarget::Remote(id("host-1"))
    );
}

#[test]
fn the_local_daemon_cannot_be_registered_as_a_remote_host() {
    assert!(check_registrable(&id("host-1")).is_ok());
    for text in ["local", "localhost"] {
        assert!(check_registrable(&id(text)).is_err(), "{text}");
    }
}
