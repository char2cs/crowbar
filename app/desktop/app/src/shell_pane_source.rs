use gpui::{AnyElement, AnyView, App, SharedString, Window};
use layout::TabId;
use protocol::PaneId;

use crate::browser_bars::BrowserBars;
use crate::dirty_panes::DirtyPanes;
use crate::native_panes::NativePanes;
use crate::serials::Serials;
use crate::titles::Titles;

pub(crate) struct ShellPaneSource {
    titles: Titles,
    serials: Serials,
    bars: BrowserBars,
    native: NativePanes,
    dirty: DirtyPanes,
}

impl ShellPaneSource {
    pub(crate) fn new(
        titles: Titles,
        serials: Serials,
        bars: BrowserBars,
        native: NativePanes,
    ) -> Self {
        Self {
            titles,
            serials,
            bars,
            native,
            dirty: DirtyPanes::default(),
        }
    }

    pub(crate) fn marking(
        mut self,
        dirty: DirtyPanes,
    ) -> Self {
        self.dirty = dirty;
        self
    }

    pub(crate) fn native(&self) -> &NativePanes {
        &self.native
    }

    pub(crate) fn bars(&self) -> &BrowserBars {
        &self.bars
    }
}

impl layoutview::PaneContent for ShellPaneSource {
    fn content(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut App,
    ) -> AnyElement {
        self.native.content(pane, window, cx)
    }

    fn is_native(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.native.is_native(pane)
    }
}

impl layoutview::PaneChrome for ShellPaneSource {
    fn chrome_height(
        &self,
        pane: &PaneId,
    ) -> f32 {
        self.bars.height(pane)
    }

    fn chrome(
        &self,
        pane: &PaneId,
        _window: &mut Window,
        cx: &mut App,
    ) -> Option<AnyView> {
        self.bars.view(pane, cx)
    }
}

impl layoutview::PaneTabs for ShellPaneSource {
    fn tab_look(
        &self,
        instance: &layout::PaneInstance,
    ) -> layoutview::TabLook {
        let title = self
            .titles
            .get(&instance.pane_id)
            .map(|title| title.to_string());
        let look = crate::tab_looks::tab_look(instance, title.as_deref());
        if self.dirty.contains(&instance.pane_id) {
            return look.unsaved();
        }
        look
    }

    fn stacks(
        &self,
        instance: &layout::PaneInstance,
    ) -> bool {
        !instance.type_id.is_chat()
    }
}

impl layoutview::PaneDetach for ShellPaneSource {
    fn label(
        &self,
        pane: &PaneId,
    ) -> SharedString {
        self.titles.get(pane).map_or_else(
            || crate::chat_label::NEW_CHAT.into(),
            |title| title.to_string().into(),
        )
    }

    fn fresh_tab(
        &self,
        pane: &PaneId,
    ) -> (TabId, String) {
        (self.serials.next_tab(), self.label(pane).to_string())
    }
}

#[cfg(test)]
mod tests {
    use layoutview::PaneDetach as _;
    use protocol::PaneTitle;

    use super::*;
    use crate::new_tab_panes::NewTabPanes;
    use crate::terminal_ids::TerminalIds;
    use crate::test_support::idle_terminals;

    fn no_bars() -> BrowserBars {
        BrowserBars::new(
            crate::browser_ids::BrowserIds::default(),
            async_channel::unbounded().0,
        )
    }

    #[test]
    fn an_unnamed_chat_is_called_new_chat_and_a_named_one_by_its_title() {
        let titles = Titles::default();
        let source = ShellPaneSource::new(
            titles.clone(),
            Serials::above(&layout::Layout::default()),
            no_bars(),
            NativePanes::new(
                idle_terminals(TerminalIds::default()),
                NewTabPanes::new(async_channel::unbounded().0),
            ),
        );
        let pane = PaneId::new("p1").unwrap();
        assert_eq!(source.label(&pane).as_ref(), "New chat", "unnamed");
        titles.set(pane.clone(), PaneTitle::new("Fix").unwrap());
        assert_eq!(source.label(&pane).as_ref(), "Fix", "named");
    }

    #[test]
    fn detached_panes_get_fresh_tabs() {
        let source = ShellPaneSource::new(
            Titles::default(),
            Serials::above(&layout::Layout::default()),
            no_bars(),
            NativePanes::new(
                idle_terminals(TerminalIds::default()),
                NewTabPanes::new(async_channel::unbounded().0),
            ),
        );
        let pane = PaneId::new("p1").unwrap();
        let (first, _) = source.fresh_tab(&pane);
        let (second, _) = source.fresh_tab(&pane);
        assert_ne!(first, second, "never the same id twice");
    }

    enum Edit {
        Type,
        Save,
    }

    impl Edit {
        fn apply(
            &self,
            dirty: &DirtyPanes,
            pane: &PaneId,
        ) {
            match self {
                Self::Type => dirty.mark(pane.clone()),
                Self::Save => dirty.clear(pane),
            }
        }
    }

    struct Case {
        name: &'static str,
        edit: Edit,
        marked: bool,
    }

    const CASES: [Case; 3] = [
        Case {
            name: "saved",
            edit: Edit::Save,
            marked: false,
        },
        Case {
            name: "edited",
            edit: Edit::Type,
            marked: true,
        },
        Case {
            name: "saved again",
            edit: Edit::Save,
            marked: false,
        },
    ];

    #[test]
    fn a_tab_is_marked_exactly_while_its_pane_has_unsaved_work() {
        use layoutview::PaneTabs as _;
        let dirty = DirtyPanes::default();
        let source = ShellPaneSource::new(
            Titles::default(),
            Serials::above(&layout::Layout::default()),
            no_bars(),
            NativePanes::new(
                idle_terminals(TerminalIds::default()),
                NewTabPanes::new(async_channel::unbounded().0),
            ),
        )
        .marking(dirty.clone());
        let pane = PaneId::new("p1").unwrap();
        let instance = layout::PaneInstance {
            pane_id: pane.clone(),
            type_id: protocol::PaneTypeId::new("core.terminal").unwrap(),
            params: protocol::Params::default(),
            state: protocol::PaneState::default(),
        };
        for case in &CASES {
            case.edit.apply(&dirty, &pane);
            let look = source.tab_look(&instance);
            assert_eq!(look.dirty, case.marked, "{}", case.name);
        }
    }
}
