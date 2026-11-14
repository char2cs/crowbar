use super::*;

#[test]
fn bundle_and_daemon_routes() {
    let git = ExtensionId::new("git").unwrap();
    let cases = [
        (
            "crowbar://local/panes/core/chat/index.html",
            Some(Route::CoreBundle {
                path: "chat/index.html".into(),
            }),
        ),
        (
            "crowbar://local/panes/core/",
            Some(Route::CoreBundle {
                path: String::new(),
            }),
        ),
        (
            "crowbar://local/panes/ext/git/review/index.html?x=1",
            Some(Route::ExtensionBundle {
                extension: git,
                path: "review/index.html".into(),
            }),
        ),
        (
            "crowbar://paired-1/v0/threads?limit=5",
            Some(Route::Daemon {
                host: HostId::new("paired-1").unwrap(),
                path_and_query: "/v0/threads?limit=5".into(),
            }),
        ),
        (
            "crowbar://local",
            Some(Route::Daemon {
                host: HostId::new("local").unwrap(),
                path_and_query: "/".into(),
            }),
        ),
    ];
    for (uri, expected) in cases {
        assert_eq!(parse("crowbar", uri), expected, "{uri}");
    }
}

#[test]
fn malformed_uris_have_no_route() {
    for uri in [
        "http://local/v0/x",
        "crowbar:/local/x",
        "crowbar://BAD HOST/x",
        "crowbar://local/panes/ext/Bad/x",
        "crowbar://local/panes/ext/git",
        "crowbar://local/panes/unknown/x",
    ] {
        assert_eq!(parse("crowbar", uri), None, "{uri}");
    }
}
