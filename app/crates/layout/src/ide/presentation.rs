pub(crate) mod editor_view;
pub(crate) mod sidebar_side;
pub(crate) mod view_mode;

use crate::ide::pane_group::PaneGroup;
use crate::ide::presentation::sidebar_side::SidebarSide;
use crate::ide::presentation::view_mode::ViewMode;
use crate::tree::axis::Axis;
use crate::tree::geometry::rect::Rect;
use crate::tree::split_node_place::divide;

/// Height of every top chrome row of a group: the chat header and the tab bar.
pub const TOP_ROW_HEIGHT: f32 = 42.0;

/// The chat's share of a group's width or height when chat and editor show together, before
/// the user drags the sash.
pub const DEFAULT_CHAT_SHARE: f32 = 0.45;

pub(crate) const SIDE_BY_SIDE_MIN: f32 = 780.0;

const SEAM: f32 = 1.0;

/// Where a group's parts sit inside its area, as develop's `usePanePresentation`. A part that is
/// hidden is `None`; the chat and the editor stay open while hidden.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Presentation {
    /// The tab bar: across the group when one view shows, else at the top of the editor.
    pub tab_bar: Option<Rect>,
    /// Whether the tab bar leads with a tab for the chat.
    pub chat_tab: bool,
    /// The chat view.
    pub chat: Option<Rect>,
    /// The editor view, below its tab bar.
    pub editor: Option<Rect>,
    /// The 1 px line on the editor's edge facing the chat when both show together.
    pub seam: Option<Rect>,
}

impl Presentation {
    /// The parts of `group` laid out in `area`, the chat taking `chat_share` of the room when it
    /// shows beside or above the editor.
    #[must_use]
    pub fn of(
        group: &PaneGroup,
        area: Rect,
        side: SidebarSide,
        chat_share: f32,
    ) -> Self {
        let mode = mode_of(group, area);
        if group.chat().is_none() {
            let (row, rest) = cut_top(area, TOP_ROW_HEIGHT);
            return Self::parts(Some(row), None, Some(rest));
        }
        if group.tabs().is_empty() && !group.editor_open() {
            return Self::parts(None, Some(area), None);
        }
        if mode == ViewMode::Tabs {
            return Self::one_at_a_time(group, area);
        }
        Self::together(mode, area, side, chat_share)
    }

    fn parts(
        tab_bar: Option<Rect>,
        chat: Option<Rect>,
        editor: Option<Rect>,
    ) -> Self {
        Self {
            tab_bar,
            chat_tab: false,
            chat,
            editor,
            seam: None,
        }
    }

    fn one_at_a_time(
        group: &PaneGroup,
        area: Rect,
    ) -> Self {
        let (row, rest) = cut_top(area, TOP_ROW_HEIGHT);
        let chat_shows = group.chat_selected();
        Self {
            chat_tab: true,
            ..Self::parts(
                Some(row),
                chat_shows.then_some(rest),
                (!chat_shows).then_some(rest),
            )
        }
    }

    fn together(
        mode: ViewMode,
        area: Rect,
        side: SidebarSide,
        chat_share: f32,
    ) -> Self {
        if mode == ViewMode::SideBySide && side == SidebarSide::Right {
            let parts = divide(area, Axis::Horizontal, 1.0 - chat_share);
            return Self::split(parts.second, parts.first);
        }
        let axis = match mode {
            ViewMode::Stacked => Axis::Vertical,
            ViewMode::SideBySide | ViewMode::Tabs => Axis::Horizontal,
        };
        let parts = divide(area, axis, chat_share);
        Self::split(parts.first, parts.second)
    }

    fn split(
        chat: Rect,
        editor_box: Rect,
    ) -> Self {
        let (seam, editor_box) = cut_seam(editor_box, chat);
        let (row, editor) = cut_top(editor_box, TOP_ROW_HEIGHT);
        Self {
            seam: Some(seam),
            ..Self::parts(Some(row), Some(chat), Some(editor))
        }
    }
}

fn mode_of(
    group: &PaneGroup,
    area: Rect,
) -> ViewMode {
    if !group.editor_open() || area.width.max(area.height) < SIDE_BY_SIDE_MIN {
        return ViewMode::Tabs;
    }
    if area.width >= area.height {
        return ViewMode::SideBySide;
    }
    ViewMode::Stacked
}

fn cut_seam(
    editor_box: Rect,
    chat: Rect,
) -> (Rect, Rect) {
    if chat.y < editor_box.y {
        return cut_top(editor_box, SEAM);
    }
    if chat.x > editor_box.x {
        return cut_right(editor_box, SEAM);
    }
    cut_left(editor_box, SEAM)
}

fn cut_left(
    area: Rect,
    width: f32,
) -> (Rect, Rect) {
    let width = width.min(area.width);
    let strip = Rect { width, ..area };
    let rest = Rect {
        x: area.x + width,
        width: area.width - width,
        ..area
    };
    (strip, rest)
}

fn cut_right(
    area: Rect,
    width: f32,
) -> (Rect, Rect) {
    let width = width.min(area.width);
    let rest = Rect {
        width: area.width - width,
        ..area
    };
    let strip = Rect {
        x: area.x + rest.width,
        width,
        ..area
    };
    (strip, rest)
}

fn cut_top(
    area: Rect,
    height: f32,
) -> (Rect, Rect) {
    let height = height.min(area.height);
    let row = Rect { height, ..area };
    let rest = Rect {
        y: area.y + height,
        height: area.height - height,
        ..area
    };
    (row, rest)
}

#[cfg(test)]
mod tests {
    use protocol::PaneId;

    use super::*;
    use crate::test_support::tab_fixtures::pane;

    const fn at(
        (x, y): (f32, f32),
        (width, height): (f32, f32),
    ) -> Rect {
        Rect {
            x,
            y,
            width,
            height,
        }
    }

    fn chat() -> PaneGroup {
        PaneGroup::holding(&pane("c"))
    }

    fn with_tab() -> PaneGroup {
        let mut group = chat();
        group.insert(PaneId::new("t").unwrap(), None).unwrap();
        group
    }

    fn chat_picked() -> PaneGroup {
        PaneGroup {
            chat_selected: true,
            ..with_tab()
        }
    }

    fn editor_closed() -> PaneGroup {
        PaneGroup {
            editor_open: false,
            ..chat_picked()
        }
    }

    fn chatless() -> PaneGroup {
        PaneGroup::single(PaneId::new("t").unwrap())
    }

    const NOTHING: Presentation = Presentation {
        tab_bar: None,
        chat_tab: false,
        chat: None,
        editor: None,
        seam: None,
    };

    const BAR: Rect = at((0.0, 0.0), (1200.0, TOP_ROW_HEIGHT));
    const SMALL_BAR: Rect = at((0.0, 0.0), (640.0, TOP_ROW_HEIGHT));

    /// A case, its group, area and sidebar side, and where the parts go.
    type Case = (&'static str, fn() -> PaneGroup, (f32, f32), SidebarSide, Presentation);

    const CASES: &[Case] = &[
        (
            "a lone chat fills the group",
            chat, (1200.0, 800.0), SidebarSide::Left, Presentation {
            chat: Some(at((0.0, 0.0), (1200.0, 800.0))),
            ..NOTHING
        }),
        (
            "wide: the chat beside a left sidebar, the editor under its tab bar",
            with_tab, (1206.0, 800.0), SidebarSide::Left, Presentation {
            chat: Some(at((0.0, 0.0), (600.0, 800.0))),
            seam: Some(at((606.0, 0.0), (1.0, 800.0))),
            tab_bar: Some(at((607.0, 0.0), (599.0, TOP_ROW_HEIGHT))),
            editor: Some(at((607.0, 42.0), (599.0, 758.0))),
            ..NOTHING
        }),
        (
            "wide: the chat beside a right sidebar",
            with_tab, (1206.0, 800.0), SidebarSide::Right, Presentation {
            tab_bar: Some(at((0.0, 0.0), (599.0, TOP_ROW_HEIGHT))),
            editor: Some(at((0.0, 42.0), (599.0, 758.0))),
            seam: Some(at((599.0, 0.0), (1.0, 800.0))),
            chat: Some(at((606.0, 0.0), (600.0, 800.0))),
            ..NOTHING
        }),
        (
            "tall: the chat stacked on top whatever the sidebar side",
            with_tab, (800.0, 1206.0), SidebarSide::Right, Presentation {
            chat: Some(at((0.0, 0.0), (800.0, 600.0))),
            seam: Some(at((0.0, 606.0), (800.0, 1.0))),
            tab_bar: Some(at((0.0, 607.0), (800.0, TOP_ROW_HEIGHT))),
            editor: Some(at((0.0, 649.0), (800.0, 557.0))),
            ..NOTHING
        }),
        (
            "small: the selected editor tab under a tab bar with a chat tab",
            with_tab, (640.0, 480.0), SidebarSide::Left, Presentation {
            tab_bar: Some(SMALL_BAR),
            chat_tab: true,
            editor: Some(at((0.0, 42.0), (640.0, 438.0))),
            ..NOTHING
        }),
        (
            "small: the selected chat under the same tab bar",
            chat_picked, (640.0, 480.0), SidebarSide::Left, Presentation {
            tab_bar: Some(SMALL_BAR),
            chat_tab: true,
            chat: Some(at((0.0, 42.0), (640.0, 438.0))),
            ..NOTHING
        }),
        (
            "a closed editor keeps its tabs behind the chat tab",
            editor_closed, (1200.0, 800.0), SidebarSide::Left, Presentation {
            tab_bar: Some(BAR),
            chat_tab: true,
            chat: Some(at((0.0, 42.0), (1200.0, 758.0))),
            ..NOTHING
        }),
        (
            "a group without a chat is its tab bar and editor",
            chatless, (1200.0, 800.0), SidebarSide::Left, Presentation {
            tab_bar: Some(BAR),
            editor: Some(at((0.0, 42.0), (1200.0, 758.0))),
            ..NOTHING
        }),
    ];

    #[test]
    fn a_group_lays_out_its_chat_and_editor_as_develop_does() {
        for &(name, group, size, side, expected) in CASES {
            let shown = Presentation::of(&group(), at((0.0, 0.0), size), side, 0.5);
            assert_eq!(shown, expected, "{name}");
        }
    }
}
