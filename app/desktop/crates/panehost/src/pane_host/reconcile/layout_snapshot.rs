use layout::Layout;
use protocol::PaneId;
use webview::ParentWindow;

use crate::pane_host::reconcile::keyboard::Keyboard;
use crate::pane_host::reconcile::overlays::Overlays;
use crate::pane_host::reconcile::pane_kinds::PaneKinds;
use crate::pane_host::reconcile::visible_pane::VisiblePane;

/// What the pane host needs from the shell to reconcile: the layout it reads panes and their
/// state from (borrowed, never copied), the window, the holes panes are shown in, the focused
/// pane, and who draws each pane.
#[derive(Debug, Clone)]
pub struct LayoutSnapshot<'a> {
    /// The model. Pane state is read from here and nowhere else.
    pub layout: &'a Layout,
    /// The native parent window.
    pub parent: ParentWindow,
    /// Panes shown in the active tab and where.
    pub visible: Vec<VisiblePane>,
    /// The focused pane of the tab the window shows.
    pub focused: Option<PaneId>,
    /// Who the keys go to while that pane is shown and nothing covers the panes.
    pub keyboard: Keyboard,
    /// What the shell draws over the panes.
    pub overlays: Overlays,
    /// Whether a page or a native view draws each pane.
    pub kinds: &'a dyn PaneKinds,
}

impl<'a> LayoutSnapshot<'a> {
    /// The snapshot of `layout` with `visible` holes in `parent`.
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

    /// The same snapshot under the shell's `overlays`.
    #[must_use]
    pub fn with_overlays(
        mut self,
        overlays: Overlays,
    ) -> Self {
        self.overlays = overlays;
        self
    }

    /// The same snapshot with `focused` as the pane that has focus: the focus of the tab the
    /// window shows, which only the shell knows.
    #[must_use]
    pub fn with_focus(
        mut self,
        focused: Option<PaneId>,
    ) -> Self {
        self.focused = focused;
        self
    }

    /// The same snapshot with the keys going to `keyboard`.
    #[must_use]
    pub fn with_keyboard(
        mut self,
        keyboard: Keyboard,
    ) -> Self {
        self.keyboard = keyboard;
        self
    }

    /// The same snapshot with `kinds` deciding who draws each pane.
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
}
