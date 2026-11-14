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
fn only_ids_a_url_authority_keeps_intact_can_be_registered() {
    assert!(check_registrable(&id("host-1")).is_ok());
    for text in ["local", "localhost", "host_1"] {
        assert!(check_registrable(&id(text)).is_err(), "{text}");
    }
}
