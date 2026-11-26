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
    use crate::catalog::PaneLifecycle;
    use crate::test_support::pane_type;

    #[test]
    fn entries_live_under_their_owner_s_prefix_tagged_with_the_page_generation() {
        let host = HostId::new("local").unwrap();
        let mut review = pane_type("ext.git.review", PaneLifecycle::Parkable);
        review.entry = "/ui/review.html".into();
        let url = entry_url("crowbar", &host, &review);
        assert_eq!(
            url, "crowbar://local/panes/ext/git/ui/review.html",
            "extension"
        );
        assert_eq!(tagged(&url, 4), format!("{url}?g=4"), "plain");
        assert_eq!(tagged("x.html?a=1", 4), "x.html?a=1&g=4", "with a query");
    }
}
