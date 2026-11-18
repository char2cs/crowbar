use layout::{Tab, TabId};
use protocol::PaneId;

use crate::pill::member_view::MemberView;

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
    pub fn from_tab(
        tab: &Tab,
        member: impl Fn(&PaneId) -> MemberView,
    ) -> Self {
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
mod tests {
    use layout::{Command, Layout, PaneInstance, TabId};
    use protocol::{PaneId, PaneState, PaneTypeId, Params};

    use super::*;

    fn instance(name: &str) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new(name).expect("id"),
            type_id: PaneTypeId::new("core.chat").expect("type"),
            params: Params::default().with("chat", name).expect("params"),
            state: PaneState::default(),
        }
    }

    fn open_pane(
        layout: &mut Layout,
        name: &str,
    ) {
        let command = Command::OpenPane {
            tab: TabId(7),
            instance: instance(name),
            placement: None,
        };
        layout.apply(command).expect("pane");
    }

    fn member(pane: &PaneId) -> MemberView {
        MemberView {
            pane: pane.clone(),
            label: pane.as_str().to_uppercase().into(),
            provisional: false,
            working: false,
            kind: crate::pill::member_kind::MemberKind::Chat,
        }
    }

    #[test]
    fn a_tab_view_lists_its_panes_in_layout_order_and_joins_their_titles() {
        let mut layout = Layout::default();
        layout
            .apply(Command::AddTab {
                id: TabId(7),
                title: "T".into(),
            })
            .expect("tab");
        for name in ["a", "b"] {
            open_pane(&mut layout, name);
        }
        let view = TabView::from_tab(layout.active_tab().expect("tab"), member);
        assert_eq!(view.id, TabId(7), "same id as the model tab");
        assert_eq!(view.members.len(), 2, "two members");
        assert_eq!(view.title(), "A | B", "titles joined");
    }
}
