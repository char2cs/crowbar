use super::*;

fn ext(id: &str) -> AccessScope {
    AccessScope::Extension(ExtensionId::new(id).unwrap())
}

#[test]
fn core_reaches_the_api_and_nothing_else() {
    for ok in ["/v0", "/v0/projects?x=1", "/v0/ext/git/call"] {
        assert!(AccessScope::Core.check(ok).is_ok(), "{ok}");
    }
    for bad in [
        "/",
        "/debug/pprof/goroutine",
        "/v00",
        "v0/x",
        "/v0/../debug",
        "/v0/%2e%2e/x",
    ] {
        assert!(AccessScope::Core.check(bad).is_err(), "{bad}");
    }
}

#[test]
fn an_extension_reaches_only_its_own_routes_and_blobs() {
    let scope = ext("acme-git");
    for ok in [
        "/v0/ext/acme-git",
        "/v0/ext/acme-git/call",
        "/v0/ext/acme-git/blobs/abc?x=1",
    ] {
        assert!(scope.check(ok).is_ok(), "{ok}");
    }
    for bad in [
        "/v0/projects",
        "/v0/ext/acme-gitx/call",
        "/v0/ext/acme/call",
        "/v0/ext/other/blobs/abc",
        "/v0/ext/",
        "/v0/ext//acme-git/call",
        "/v0/ext/acme-git/../other/call",
        "/v0/ext/acme-git/%2E%2E/other",
        "/v0/ext/acme-git/..%2fother",
        "/v0/ext/acme-git\\..\\other",
        "/v0/ext/acme-git/a\u{0}b",
        "/v0/ext/acme-git/.",
    ] {
        assert!(scope.check(bad).is_err(), "{bad}");
    }
}

#[test]
fn a_query_cannot_smuggle_another_route_in() {
    assert!(ext("a").check("/v0/projects?/v0/ext/a/x").is_err());
    assert!(ext("a").check("/v0/ext/a/x?/../../projects").is_ok());
}

#[test]
fn the_scope_follows_the_pane_kind() {
    let id = ExtensionId::new("acme").unwrap();
    assert_eq!(
        AccessScope::of(PaneKind::Core, None).unwrap(),
        AccessScope::Core
    );
    assert_eq!(
        AccessScope::of(PaneKind::Extension, Some(&id)).unwrap(),
        ext("acme")
    );
    assert!(AccessScope::of(PaneKind::Core, Some(&id)).is_err());
    assert!(AccessScope::of(PaneKind::Extension, None).is_err());
}
