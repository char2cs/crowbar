use super::*;

fn remote(id: &str) -> HostTarget {
    HostTarget::Remote(HostId::new(id).unwrap())
}

#[test]
fn qualifying_matches_the_panes() {
    assert_eq!(qualify(&HostTarget::Local, "/v0/x").unwrap(), "/v0/x");
    assert_eq!(
        qualify(&remote("host-1"), "/v0/x?a=1").unwrap(),
        "crowbar://host-1/v0/x?a=1"
    );
    for path in ["crowbar://host-1/v0", " CROWBAR://x/v0"] {
        assert!(qualify(&HostTarget::Local, path).is_err(), "{path}");
    }
    for path in ["v0/x", "//evil/v0", ""] {
        assert!(qualify(&remote("host-1"), path).is_err(), "{path}");
    }
}

#[test]
fn splitting_matches_the_panes() {
    let split = |input: &str| split_host_path(input).unwrap();
    assert_eq!(split("/v0/x").target, HostTarget::Local);
    assert_eq!(split("crowbar://localhost/v0/x").path, "/v0/x");
    assert_eq!(split("crowbar://localhost/v0/x").target, HostTarget::Local);
    let qualified = split("crowbar://host-1/v0/x?y=1");
    assert_eq!(qualified.target, remote("host-1"));
    assert_eq!(qualified.path, "/v0/x?y=1");
    assert_eq!(split("crowbar://editor/x").target, remote("editor"));
}

#[test]
fn anything_ambiguous_is_refused() {
    for input in [
        "Crowbar://host-1/v0",
        " crowbar://host-1/v0",
        "crowbar://localhost:80/v0",
        "crowbar:///v0",
        "crowbar://local/v0",
        "crowbar://Host-1/v0",
        "crowbar://host-1",
        "crowbar://host-1//evil",
    ] {
        assert!(split_host_path(input).is_err(), "{input}");
    }
}
