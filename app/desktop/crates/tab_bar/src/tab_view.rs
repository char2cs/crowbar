use model::{Tab, TabId};
use protocol::PaneId;

use crate::member_view::MemberView;

/// One tab of the strip: a single chat, or a set of chats that share one pill.
#[derive(Debug, Clone, PartialEq)]
pub struct TabView {
    /// The model tab it stands for.
    pub id: TabId,
    /// The chats of the tab in layout order.
    pub members: Vec<MemberView>,
}

impl TabView {
    /// The view of `tab`, with `member` supplying how each of its panes looks.
    #[must_use]
    pub fn from_tab(tab: &Tab, member: impl Fn(&PaneId) -> MemberView) -> Self {
        Self {
            id: tab.id(),
            members: tab.pane_ids().iter().map(member).collect(),
        }
    }

    /// The text of the drag ghost and of accessibility labels: the members' titles joined.
    #[must_use]
    pub fn title(&self) -> String {
        let titles: Vec<&str> = self.members.iter().map(|m| m.label.as_ref()).collect();
        titles.join(" | ")
    }
}

#[cfg(test)]
#[path = "tab_view_tests.rs"]
mod tests;
