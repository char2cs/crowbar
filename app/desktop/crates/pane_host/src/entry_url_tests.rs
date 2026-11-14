use super::*;
use crate::multiplicity::Multiplicity;
use crate::pane_lifecycle::PaneLifecycle;

fn pane_type(id: &str, entry: &str) -> PaneType {
    PaneType {
        id: PaneTypeId::new(id).unwrap(),
        title: String::new(),
        entry: entry.into(),
        lifecycle: PaneLifecycle::Parkable,
        multiplicity: Multiplicity::Single,
    }
}

#[test]
fn core_and_extension_entries_live_under_their_own_prefix() {
    let host = HostId::new("local").unwrap();
    assert_eq!(
        entry_url("crowbar", &host, &pane_type("core.chat", "chat/index.html")),
        "crowbar://local/panes/core/chat/index.html",
        "core"
    );
    assert_eq!(
        entry_url(
            "crowbar",
            &host,
            &pane_type("ext.git.review", "/ui/review.html")
        ),
        "crowbar://local/panes/ext/git/ui/review.html",
        "extension"
    );
}
