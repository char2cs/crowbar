use layout::Layout;
use protocol::PaneId;
use webview::ParentWindow;

use crate::keyboard::Keyboard;
use crate::visible_pane::VisiblePane;

/// What the pane host needs from the shell to reconcile: the layout it reads panes and their
/// state from (borrowed, never copied), the window, the holes panes are shown in, and the
/// focused pane.
#[derive(Debug, Clone)]
pub struct LayoutSnapshot<'a> {
    /// The model. Pane state is read from here and nowhere else.
    pub layout: &'a Layout,
    /// The native parent window.
    pub parent: ParentWindow,
    /// Panes shown in the active tab and where.
    pub visible: Vec<VisiblePane>,
    /// The focused pane of the active tab.
    pub focused: Option<PaneId>,
    /// Who the keys go to while that pane is shown.
    pub keyboard: Keyboard,
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
            focused: layout.active_tab().and_then(|tab| tab.focused().cloned()),
            keyboard: Keyboard::default(),
        }
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
            },
            open(1, "p1"),
            open(1, "p2"),
            Command::AddTab {
                id: TabId(2),
                title: "b".into(),
            },
            open(2, "p3"),
            Command::ActivateTab { tab: TabId(1) },
        ] {
            layout.apply(command).unwrap();
        }
        layout
    }

    #[test]
    fn lists_every_pane_and_the_focus_of_the_active_tab() {
        let hole = VisiblePane {
            pane: PaneId::new("p1").unwrap(),
            bounds: bounds(),
        };
        let layout = layout();
        let snapshot =
            LayoutSnapshot::from_layout(&layout, ParentWindow::X11(5), vec![hole.clone()]);
        assert_eq!(snapshot.pane_ids().count(), 3, "all tabs");
        assert_eq!(snapshot.visible, [hole], "the holes the shell gave");
        assert_eq!(
            snapshot.focused.as_ref().map(PaneId::as_str),
            Some("p2"),
            "focus of the active tab"
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
