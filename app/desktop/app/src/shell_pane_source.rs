use gpui::{AnyElement, AnyView, App, IntoElement as _, Styled as _, Window};
use protocol::PaneId;

use crate::browser_bars::BrowserBars;
use crate::dirty_panes::DirtyPanes;
use crate::native_panes::NativePanes;
use crate::serials::Serials;
use crate::titles::Titles;

mod branch_review;
mod file_icon;
mod pane_detach;
mod read_only;

pub(crate) type ChatGlyphs = std::rc::Rc<dyn Fn(&str) -> Option<String>>;
pub(crate) type ChatReviews = std::rc::Rc<dyn Fn(&str) -> bool>;
pub(crate) type ChatLocks = std::rc::Rc<dyn Fn(&str) -> bool>;
pub(crate) type FileIcons = std::rc::Rc<dyn Fn() -> sidebar::IconTheme>;

pub(crate) struct ShellPaneSource {
    titles: Titles,
    serials: Serials,
    bars: BrowserBars,
    native: NativePanes,
    dirty: DirtyPanes,
    glyphs: ChatGlyphs,
    reviews: ChatReviews,
    locks: ChatLocks,
    icons: (FileIcons, std::rc::Rc<ui::MaterialTable>),
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
            glyphs: std::rc::Rc::new(|_| None),
            reviews: std::rc::Rc::new(|_| false),
            locks: std::rc::Rc::new(|_| false),
            icons: (
                std::rc::Rc::new(sidebar::IconTheme::default),
                std::rc::Rc::default(),
            ),
        }
    }

    pub(crate) fn drawing(
        mut self,
        glyphs: ChatGlyphs,
    ) -> Self {
        self.glyphs = glyphs;
        self
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
        let palette = *ui::Theme::of(cx).palette(ui::Surface::Window);
        if let Some(fill) = self.bars.ground(pane, &palette) {
            return gpui::div().size_full().bg(fill).into_any_element();
        }
        self.native.content(pane, window, cx)
    }

    fn is_native(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.native.is_native(pane)
    }

    fn empty_editor(
        &self,
        chat: &PaneId,
        _window: &mut Window,
        cx: &mut App,
    ) -> AnyElement {
        self.native.empty_editor(chat, cx)
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

impl ideshell::TabLooks for ShellPaneSource {
    fn tab_look(
        &self,
        instance: &layout::PaneInstance,
    ) -> ideshell::TabLook {
        let title = self
            .titles
            .get(&instance.pane_id)
            .map(|title| title.to_string());
        let mut look = self.file_icon(
            instance,
            crate::tab_looks::tab_look(instance, title.as_deref()),
        );
        if let Some(artwork) =
            crate::tab_looks::chat_of(instance).and_then(|chat| (self.glyphs)(chat))
        {
            look = look.drawn(artwork);
        }
        if self.locked(instance) {
            look = look.read_only();
        }
        if self.dirty.contains(&instance.pane_id) {
            return look.unsaved();
        }
        look
    }
}

#[cfg(test)]
mod tests {
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
    fn a_chat_tab_draws_its_provider_s_artwork_once_it_is_known_and_other_tabs_never_do() {
        use ideshell::TabLooks as _;
        const MARK: &str = "<svg/>";
        let source = ShellPaneSource::new(
            Titles::default(),
            Serials::above(&layout::Layout::default()),
            no_bars(),
            NativePanes::new(
                idle_terminals(TerminalIds::default()),
                NewTabPanes::new(async_channel::unbounded().0),
            ),
        )
        .drawing(std::rc::Rc::new(|chat| {
            (chat == "c1").then(|| MARK.to_owned())
        }));
        let pane = |kind: &str, thread: &str| layout::PaneInstance {
            pane_id: PaneId::new("p1").unwrap(),
            type_id: protocol::PaneTypeId::new(kind).unwrap(),
            params: protocol::Params::default()
                .with("threadId", thread)
                .unwrap(),
            state: protocol::PaneState::default(),
        };
        let cases = [
            ("core.chat", "c1", Some(MARK)),
            ("core.chat", "c2", None),
            ("core.terminal", "c1", None),
        ];
        for (kind, thread, artwork) in cases {
            let look = source.tab_look(&pane(kind, thread));
            let drawn = look.artwork.map(|svg| svg.to_string());
            assert_eq!(drawn.as_deref(), artwork, "{kind} {thread}");
        }
    }

    #[test]
    fn a_tab_is_marked_exactly_while_its_pane_has_unsaved_work() {
        use ideshell::TabLooks as _;
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
