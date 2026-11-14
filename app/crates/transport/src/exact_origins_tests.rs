use protocol::{ExtensionId, HostId, PaneId, PaneOrigin};

use super::*;

fn scope(extension: Option<&str>) -> RelayScope {
    let origin = extension.map_or(PaneOrigin::Core, |id| {
        PaneOrigin::Extension(ExtensionId::new(id).unwrap())
    });
    RelayScope::new(
        HostId::new("local").unwrap(),
        PaneId::new("p1").unwrap(),
        origin,
    )
}

fn policy() -> ExactOrigins {
    ExactOrigins::new(
        vec!["crowbar-pane://app".into()],
        "crowbar-ext://{extension}",
    )
}

#[test]
fn a_core_pane_may_present_only_a_core_origin() {
    let core = scope(None);
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
    let acme = scope(Some("acme"));
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
