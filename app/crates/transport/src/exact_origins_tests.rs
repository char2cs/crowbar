use protocol::{ExtensionId, HostId, PaneId};

use super::*;

fn scope(kind: PaneKind, extension: Option<&str>) -> RelayScope {
    RelayScope {
        host: HostId::new("local").unwrap(),
        pane: PaneId::new("p1").unwrap(),
        kind,
        extension: extension.map(|id| ExtensionId::new(id).unwrap()),
    }
}

fn policy() -> ExactOrigins {
    ExactOrigins::new(
        vec!["crowbar-pane://app".into()],
        "crowbar-ext://{extension}",
    )
}

#[test]
fn a_core_pane_may_present_only_a_core_origin() {
    let core = scope(PaneKind::Core, None);
    assert!(policy().allows(&core, "crowbar-pane://app"));
    for origin in [
        "",
        "null",
        "crowbar-pane://app/",
        "crowbar-pane://app.evil",
        "http://127.0.0.1:1",
        "crowbar-ext://acme",
    ] {
        assert!(!policy().allows(&core, origin), "{origin}");
    }
}

#[test]
fn an_extension_pane_may_present_only_its_own_origin() {
    let acme = scope(PaneKind::Extension, Some("acme"));
    assert!(policy().allows(&acme, "crowbar-ext://acme"));
    for origin in [
        "crowbar-ext://other",
        "crowbar-ext://acme2",
        "crowbar-pane://app",
        "null",
    ] {
        assert!(!policy().allows(&acme, origin), "{origin}");
    }
}

#[test]
fn a_scope_that_contradicts_itself_is_never_allowed() {
    assert!(!policy().allows(&scope(PaneKind::Extension, None), "crowbar-pane://app"));
    assert!(!policy().allows(&scope(PaneKind::Core, Some("acme")), "crowbar-pane://app"));
}
