pub(crate) mod editor_view;
pub(crate) mod sidebar_side;
pub(crate) mod view_mode;

use crate::ide::pane_group::PaneGroup;
use crate::ide::presentation::sidebar_side::SidebarSide;
use crate::ide::presentation::view_mode::ViewMode;
use crate::tree::axis::Axis;
use crate::tree::geometry::rect::Rect;
use crate::tree::split_node::SASH_THICKNESS;
use crate::tree::split_node_place::divide;

pub const TOP_ROW_HEIGHT: f32 = 42.0;

const CHAT_MIN_BESIDE: f32 = 340.0;
const CHAT_MIN_STACKED: f32 = 160.0;

pub const SIDE_BY_SIDE_MIN: f32 = 780.0;

const SEAM: f32 = 1.0;

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Presentation {
    pub mode: ViewMode,
    pub chat_header: Option<Rect>,
    pub tab_bar: Option<Rect>,
    pub chat_tab: bool,
    pub chat: Option<Rect>,
    pub sash: Option<Rect>,
    pub editor: Option<Rect>,
    pub seam: Option<Rect>,
}

impl Presentation {
    #[must_use]
    pub fn of(
        group: &PaneGroup,
        area: Rect,
        side: SidebarSide,
    ) -> Self {
        let mode = mode_of(group, area);
        if group.chat().is_none() {
            return Self::editor_only(mode, area);
        }
        if group.tabs().is_empty() && !group.editor_open() {
            return Self::chat_only(mode, area);
        }
        if mode == ViewMode::Tabs {
            return Self::one_at_a_time(group, area);
        }
        Self::together(mode, area, side, group.chat_share())
    }

    #[must_use]
    pub fn chat_share_for(
        mode: ViewMode,
        area: Rect,
        wanted: f32,
    ) -> f32 {
        let (total, min) = match mode {
            ViewMode::Stacked => (area.height, CHAT_MIN_STACKED),
            ViewMode::SideBySide | ViewMode::Tabs => (area.width, CHAT_MIN_BESIDE),
        };
        let room = (total - SASH_THICKNESS).max(0.0);
        if room < 2.0 * min {
            return 0.5;
        }
        wanted.clamp(min, room - min) / room
    }

    fn editor_only(
        mode: ViewMode,
        area: Rect,
    ) -> Self {
        let (row, rest) = cut_top(area, TOP_ROW_HEIGHT);
        Self {
            mode,
            chat_header: None,
            tab_bar: Some(row),
            chat_tab: false,
            chat: None,
            sash: None,
            editor: Some(rest),
            seam: None,
        }
    }

    fn chat_only(
        mode: ViewMode,
        area: Rect,
    ) -> Self {
        let (row, _) = cut_top(area, TOP_ROW_HEIGHT);
        Self {
            mode,
            chat_header: Some(row),
            tab_bar: None,
            chat_tab: false,
            chat: Some(area),
            sash: None,
            editor: None,
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
            mode: ViewMode::Tabs,
            chat_header: None,
            tab_bar: Some(row),
            chat_tab: true,
            chat: chat_shows.then_some(rest),
            sash: None,
            editor: (!chat_shows).then_some(rest),
            seam: None,
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
            return Self::split(mode, parts.second, parts.sash, parts.first);
        }
        let axis = match mode {
            ViewMode::Stacked => Axis::Vertical,
            ViewMode::SideBySide | ViewMode::Tabs => Axis::Horizontal,
        };
        let parts = divide(area, axis, chat_share);
        Self::split(mode, parts.first, parts.sash, parts.second)
    }

    fn split(
        mode: ViewMode,
        chat: Rect,
        sash: Rect,
        editor_box: Rect,
    ) -> Self {
        let (seam, editor_box) = cut_seam(editor_box, chat);
        let (row, editor) = cut_top(editor_box, TOP_ROW_HEIGHT);
        Self {
            mode,
            chat_header: None,
            tab_bar: Some(row),
            chat_tab: false,
            chat: Some(chat),
            sash: Some(sash),
            editor: Some(editor),
            seam: Some(seam),
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
    use protocol::{PaneId, PaneState, PaneTypeId, Params};

    use super::*;
    use crate::layout::tab::pane_instance::PaneInstance;

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
        PaneGroup::holding(&PaneInstance {
            pane_id: PaneId::new("c").unwrap(),
            type_id: PaneTypeId::new(PaneTypeId::CHAT).unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        })
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

    struct Case {
        name: &'static str,
        group: fn() -> PaneGroup,
        size: (f32, f32),
        side: SidebarSide,
        expected: Presentation,
    }

    const NOTHING: Presentation = Presentation {
        mode: ViewMode::Tabs,
        chat_header: None,
        tab_bar: None,
        chat_tab: false,
        chat: None,
        sash: None,
        editor: None,
        seam: None,
    };

    const CASES: &[Case] = &[
        Case {
            name: "a lone chat fills the group under its own header",
            group: chat,
            size: (1200.0, 800.0),
            side: SidebarSide::Left,
            expected: Presentation {
                chat_header: Some(at((0.0, 0.0), (1200.0, TOP_ROW_HEIGHT))),
                chat: Some(at((0.0, 0.0), (1200.0, 800.0))),
                ..NOTHING
            },
        },
        Case {
            name: "wide: the chat beside a left sidebar, the editor under its tab bar",
            group: with_tab,
            size: (1206.0, 800.0),
            side: SidebarSide::Left,
            expected: Presentation {
                mode: ViewMode::SideBySide,
                chat: Some(at((0.0, 0.0), (600.0, 800.0))),
                sash: Some(at((600.0, 0.0), (6.0, 800.0))),
                seam: Some(at((606.0, 0.0), (1.0, 800.0))),
                tab_bar: Some(at((607.0, 0.0), (599.0, TOP_ROW_HEIGHT))),
                editor: Some(at((607.0, 42.0), (599.0, 758.0))),
                ..NOTHING
            },
        },
        Case {
            name: "wide: the chat beside a right sidebar",
            group: with_tab,
            size: (1206.0, 800.0),
            side: SidebarSide::Right,
            expected: Presentation {
                mode: ViewMode::SideBySide,
                tab_bar: Some(at((0.0, 0.0), (599.0, TOP_ROW_HEIGHT))),
                editor: Some(at((0.0, 42.0), (599.0, 758.0))),
                seam: Some(at((599.0, 0.0), (1.0, 800.0))),
                sash: Some(at((600.0, 0.0), (6.0, 800.0))),
                chat: Some(at((606.0, 0.0), (600.0, 800.0))),
                ..NOTHING
            },
        },
        Case {
            name: "tall: the chat stacked on top whatever the sidebar side",
            group: with_tab,
            size: (800.0, 1206.0),
            side: SidebarSide::Right,
            expected: Presentation {
                mode: ViewMode::Stacked,
                chat: Some(at((0.0, 0.0), (800.0, 600.0))),
                sash: Some(at((0.0, 600.0), (800.0, 6.0))),
                seam: Some(at((0.0, 606.0), (800.0, 1.0))),
                tab_bar: Some(at((0.0, 607.0), (800.0, TOP_ROW_HEIGHT))),
                editor: Some(at((0.0, 649.0), (800.0, 557.0))),
                ..NOTHING
            },
        },
        Case {
            name: "small: the selected editor tab under a tab bar with a chat tab",
            group: with_tab,
            size: (640.0, 480.0),
            side: SidebarSide::Left,
            expected: Presentation {
                tab_bar: Some(at((0.0, 0.0), (640.0, TOP_ROW_HEIGHT))),
                chat_tab: true,
                editor: Some(at((0.0, 42.0), (640.0, 438.0))),
                ..NOTHING
            },
        },
        Case {
            name: "small: the selected chat under the same tab bar",
            group: chat_picked,
            size: (640.0, 480.0),
            side: SidebarSide::Left,
            expected: Presentation {
                tab_bar: Some(at((0.0, 0.0), (640.0, TOP_ROW_HEIGHT))),
                chat_tab: true,
                chat: Some(at((0.0, 42.0), (640.0, 438.0))),
                ..NOTHING
            },
        },
        Case {
            name: "a closed editor keeps its tabs behind the chat tab",
            group: editor_closed,
            size: (1200.0, 800.0),
            side: SidebarSide::Left,
            expected: Presentation {
                tab_bar: Some(at((0.0, 0.0), (1200.0, TOP_ROW_HEIGHT))),
                chat_tab: true,
                chat: Some(at((0.0, 42.0), (1200.0, 758.0))),
                ..NOTHING
            },
        },
        Case {
            name: "a group without a chat is its tab bar and editor",
            group: chatless,
            size: (1200.0, 800.0),
            side: SidebarSide::Left,
            expected: Presentation {
                mode: ViewMode::SideBySide,
                tab_bar: Some(at((0.0, 0.0), (1200.0, TOP_ROW_HEIGHT))),
                editor: Some(at((0.0, 42.0), (1200.0, 758.0))),
                ..NOTHING
            },
        },
    ];

    fn halved(group: PaneGroup) -> PaneGroup {
        PaneGroup {
            chat_share: Some(0.5),
            ..group
        }
    }

    #[test]
    fn a_dragged_chat_keeps_both_sides_at_develop_s_minimum() {
        let cases = [
            ("beside", ViewMode::SideBySide, (1006.0, 800.0), 600.0, 0.6),
            (
                "too narrow",
                ViewMode::SideBySide,
                (1006.0, 800.0),
                100.0,
                0.34,
            ),
            (
                "too wide",
                ViewMode::SideBySide,
                (1006.0, 800.0),
                900.0,
                0.66,
            ),
            ("stacked", ViewMode::Stacked, (800.0, 1006.0), 50.0, 0.16),
            ("no room", ViewMode::SideBySide, (506.0, 800.0), 100.0, 0.5),
        ];
        for (name, mode, size, wanted, share) in cases {
            let area = at((0.0, 0.0), size);
            let got = Presentation::chat_share_for(mode, area, wanted);
            assert!((got - share).abs() < 1e-6, "{name}: {got}");
        }
    }

    #[test]
    fn a_group_lays_out_its_chat_and_editor_as_develop_does() {
        for case in CASES {
            let area = at((0.0, 0.0), case.size);
            let shown = Presentation::of(&halved((case.group)()), area, case.side);
            assert_eq!(shown, case.expected, "{}", case.name);
        }
    }
}
