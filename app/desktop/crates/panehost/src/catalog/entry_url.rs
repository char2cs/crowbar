use protocol::{HostId, PaneOrigin, PaneTypeId};

use crate::catalog::pane_type::PaneType;

pub(crate) fn entry_url(
    scheme: &str,
    host: &HostId,
    pane_type: &PaneType,
) -> String {
    let entry = pane_type.entry.trim_start_matches('/');
    match owner_segment(&pane_type.id) {
        Some(extension) => format!("{scheme}://{host}/panes/ext/{extension}/{entry}"),
        None => format!("{scheme}://{host}/panes/core/{entry}"),
    }
}

fn owner_segment(id: &PaneTypeId) -> Option<String> {
    match id.origin() {
        PaneOrigin::Core => None,
        PaneOrigin::Extension(extension) => Some(extension.to_string()),
    }
}

const GENERATION_PARAM: &str = "g";

pub(crate) fn tagged(
    url: &str,
    generation: u64,
) -> String {
    if url.contains('?') {
        return format!("{url}&{GENERATION_PARAM}={generation}");
    }
    format!("{url}?{GENERATION_PARAM}={generation}")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::catalog::multiplicity::Multiplicity;
    use crate::catalog::pane_lifecycle::PaneLifecycle;

    fn pane_type(
        id: &str,
        entry: &str,
    ) -> PaneType {
        PaneType {
            id: PaneTypeId::new(id).unwrap(),
            title: String::new(),
            entry: entry.into(),
            kind: crate::catalog::pane_kind::PaneKind::Web,
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

    #[test]
    fn urls_are_tagged_with_the_page_generation() {
        assert_eq!(
            tagged("crowbar://local/panes/core/x.html", 4),
            "crowbar://local/panes/core/x.html?g=4",
            "plain"
        );
        assert_eq!(
            tagged("crowbar://local/x.html?a=1", 4),
            "crowbar://local/x.html?a=1&g=4",
            "with a query"
        );
    }
}
