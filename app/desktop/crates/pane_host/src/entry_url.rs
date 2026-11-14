use protocol::{HostId, PaneKind, PaneTypeId};

use crate::pane_type::PaneType;

/// The URL a pane type's entry page is loaded from: `<scheme>://<host>/panes/core/<entry>` or
/// `<scheme>://<host>/panes/ext/<extension>/<entry>`.
pub(crate) fn entry_url(scheme: &str, host: &HostId, pane_type: &PaneType) -> String {
    let entry = pane_type.entry.trim_start_matches('/');
    match owner_segment(&pane_type.id) {
        Some(extension) => format!("{scheme}://{host}/panes/ext/{extension}/{entry}"),
        None => format!("{scheme}://{host}/panes/core/{entry}"),
    }
}

fn owner_segment(id: &PaneTypeId) -> Option<String> {
    match id.kind() {
        PaneKind::Core => None,
        PaneKind::Extension => id.extension().map(|extension| extension.to_string()),
    }
}

#[cfg(test)]
#[path = "entry_url_tests.rs"]
mod tests;
