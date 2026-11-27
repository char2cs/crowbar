use layout::Layout;
use protocol::PaneId;
use webview::ParentWindow;

use crate::pane_host::reconcile::keyboard::Keyboard;
use crate::pane_host::reconcile::overlays::Overlays;
use crate::pane_host::reconcile::pane_kinds::PaneKinds;
use crate::pane_host::reconcile::visible_pane::VisiblePane;

#[derive(Debug, Clone)]
pub struct LayoutSnapshot<'a> {
    pub layout: &'a Layout,
    pub parent: ParentWindow,
    pub visible: Vec<VisiblePane>,
    pub focused: Option<PaneId>,
    pub keyboard: Keyboard,
    pub overlays: Overlays,
    pub kinds: &'a dyn PaneKinds,
}

impl<'a> LayoutSnapshot<'a> {
    #[must_use]
    pub fn from_layout(
        layout: &'a Layout,
        parent: ParentWindow,
        visible: Vec<VisiblePane>,
    ) -> Self {
        Self {
            layout,
            parent,
            visible,
            focused: None,
            keyboard: Keyboard::default(),
            overlays: Overlays::default(),
            kinds: &(),
        }
    }

    #[must_use]
    pub fn with_overlays(
        mut self,
        overlays: Overlays,
    ) -> Self {
        self.overlays = overlays;
        self
    }

    #[must_use]
    pub fn with_focus(
        mut self,
        focused: Option<PaneId>,
    ) -> Self {
        self.focused = focused;
        self
    }

    #[must_use]
    pub fn with_keyboard(
        mut self,
        keyboard: Keyboard,
    ) -> Self {
        self.keyboard = keyboard;
        self
    }

    #[must_use]
    pub fn with_kinds(
        mut self,
        kinds: &'a dyn PaneKinds,
    ) -> Self {
        self.kinds = kinds;
        self
    }

    pub(crate) fn pane_ids(&self) -> impl Iterator<Item = PaneId> + '_ {
        self.layout.tabs().iter().flat_map(layout::Tab::pane_ids)
    }
}

#[cfg(test)]
mod tests {
    use layout::{Command, PaneInstance, TabId};
    use protocol::{PaneState, PaneTypeId, Params};
    use webview::Bounds;

    use super::*;

    fn chat(id: &str) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new(id).unwrap(),
            type_id: PaneTypeId::new("core.chat").unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        }
    }

    fn open(
        tab: u64,
        id: &str,
    ) -> Command {
        Command::OpenPane {
            tab: TabId(tab),
            instance: chat(id),
            placement: None,
        }
    }

    fn bounds() -> Bounds {
        Bounds {
            x: 1.0,
            y: 2.0,
            width: 3.0,
            height: 4.0,
        }
    }

    fn layout() -> Layout {
        let mut layout = Layout::default();
        for command in [
            Command::AddTab {
                id: TabId(1),
                title: "a".into(),
                space: layout::SpaceId::default(),
            },
            open(1, "p1"),
            open(1, "p2"),
            Command::AddTab {
                id: TabId(2),
                title: "b".into(),
                space: layout::SpaceId::default(),
            },
            open(2, "p3"),
            Command::ActivateTab { tab: TabId(1) },
        ] {
            layout.apply(command).unwrap();
        }
        layout
    }

    #[test]
    fn lists_every_pane_and_the_focus_it_was_given() {
        let hole = VisiblePane {
            pane: PaneId::new("p1").unwrap(),
            bounds: bounds(),
            corners: webview::Corners::default(),
        };
        let layout = layout();
        let focus = layout.showing("").and_then(|tab| tab.focused().cloned());
        let snapshot =
            LayoutSnapshot::from_layout(&layout, ParentWindow::X11(5), vec![hole.clone()])
                .with_focus(focus);
        assert_eq!(snapshot.pane_ids().count(), 3, "all tabs");
        assert_eq!(snapshot.visible, [hole], "the holes the shell gave");
        assert_eq!(
            snapshot.focused.as_ref().map(PaneId::as_str),
            Some("p2"),
            "the focus it was given"
        );
    }

    #[test]
    fn an_empty_layout_has_nothing_to_show() {
        let layout = Layout::default();
        let snapshot = LayoutSnapshot::from_layout(&layout, ParentWindow::X11(1), Vec::new());
        assert!(
            snapshot.pane_ids().next().is_none()
                && snapshot.visible.is_empty()
                && snapshot.focused.is_none(),
            "empty"
        );
    }
}
