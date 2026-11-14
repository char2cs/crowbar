use protocol::{ExtensionId, HostId};

use super::*;

fn extension(name: &str) -> Origin {
    Origin {
        kind: PaneKind::Extension,
        extension: Some(ExtensionId::new(name).unwrap()),
    }
}

fn daemon(path: &str) -> Route {
    Route::Daemon {
        host: HostId::new("local").unwrap(),
        path_and_query: path.into(),
    }
}

#[test]
fn core_panes_reach_the_core_bundle_and_the_whole_daemon() {
    let core = Origin {
        kind: PaneKind::Core,
        extension: None,
    };
    assert!(
        allows(
            &core,
            &Route::CoreBundle {
                path: String::new()
            }
        ),
        "core bundle"
    );
    assert!(allows(&core, &daemon("/v0/threads")), "daemon");
    let other = Route::ExtensionBundle {
        extension: ExtensionId::new("git").unwrap(),
        path: String::new(),
    };
    assert!(!allows(&core, &other), "extension bundle");
}

#[test]
fn extension_panes_reach_only_their_own_bundle_and_routes() {
    let git = extension("git");
    let own = Route::ExtensionBundle {
        extension: ExtensionId::new("git").unwrap(),
        path: "a".into(),
    };
    let foreign = Route::ExtensionBundle {
        extension: ExtensionId::new("draw").unwrap(),
        path: "a".into(),
    };
    assert!(allows(&git, &own), "own bundle");
    assert!(!allows(&git, &foreign), "foreign bundle");
    assert!(
        !allows(
            &git,
            &Route::CoreBundle {
                path: String::new()
            }
        ),
        "core bundle"
    );
    assert!(
        allows(&git, &daemon("/v0/ext/git/review.diff")),
        "own routes"
    );
    assert!(!allows(&git, &daemon("/v0/ext/gitx/x")), "prefix lookalike");
    assert!(!allows(&git, &daemon("/v0/ext/draw/x")), "foreign routes");
    assert!(!allows(&git, &daemon("/v0/threads")), "general api");
}

#[test]
fn an_extension_origin_without_an_extension_gets_nothing() {
    let nameless = Origin {
        kind: PaneKind::Extension,
        extension: None,
    };
    assert!(!allows(&nameless, &daemon("/v0/ext/git/x")), "no extension");
}
