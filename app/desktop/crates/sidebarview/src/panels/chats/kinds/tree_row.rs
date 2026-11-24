//! A normal row of the chats tree.

mod drag_payload;
mod editable_mark;

use std::rc::Rc;

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ClickEvent, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, SharedString, Stateful, StatefulInteractiveElement, Styled, WeakEntity, Window,
    div, px,
};
use sidebar::{Expansion, Presence, Row, RowId};
use ui::{FlickerSpinner, Icon, Palette, Pose, Surface};

use crate::column::mark_view;
use crate::panels::chats::parts::ClusterSpec;
use crate::panels::chats::parts::RowBounds;
use crate::panels::chats::parts::RowGlyph;
use crate::panels::chats::parts::RowInput;
use crate::panels::chats::parts::Trailing;
use crate::panels::chats::parts::cluster;
use crate::panels::chats::parts::label;
use crate::panels::chats::parts::row_shell;
use crate::panels::chats::parts::{hovered_by, row_hover};
use crate::panels::chats::parts::{ringed, row_focus};
use crate::panels::chats::reorder::GhostBuild;
use crate::panels::chats::reorder::draggable;
use crate::panels::chats::reorder::{RowDrag, tracked};
use crate::sidebar::{RowAction, RowHandler, Sidebar};

const GLYPH: f32 = 16.0;
const HEADER_GLYPH: f32 = 20.0;
const SPINNER: f32 = 14.0;

#[derive(IntoElement)]
pub(crate) struct TreeRow {
    pub(crate) row: Row,
    pub(crate) expansion: Expansion,
    pub(crate) input: Option<RowInput>,
    pub(crate) handler: RowHandler,
    pub(crate) drag: RowDrag,
    pub(crate) bounds: RowBounds,
    pub(crate) width: f32,
    pub(crate) icon_editing: Option<WeakEntity<Sidebar>>,
}

impl TreeRow {
    fn shows_buttons(
        &self,
        pose: Pose,
    ) -> bool {
        let kept = buttons_shown(self.input.as_ref().map(|input| input.trailing));
        kept && self.drag.ghost.is_none() && pose == Pose::On
    }
}

impl RenderOnce for TreeRow {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let theme = self.drag.theme(cx);
        let palette = theme.palette(Surface::Chrome);
        let group = SharedString::from(format!("row-{}", self.row.id.as_str()));
        let idle = palette.sidebar_element_idle();
        let spec = ClusterSpec {
            row: &self.row,
            group: group.clone(),
            expansion: self.expansion,
            palette,
            handler: Rc::clone(&self.handler),
        };
        let id = ElementId::from(group.clone());
        let focus = row_focus(&id, window, cx);
        let hover = row_hover(&id, window, cx);
        let live = self.input.is_none() && self.drag.ghost.is_none();
        let buttons = self.shows_buttons(*hover.read(cx)).then(|| cluster(&spec));
        let shell = row_shell(id, self.row.depth, palette);
        let shell = ringed(hovered_by(shell, &hover), &focus, palette)
            .group(group)
            .hover(move |style| style.bg(idle))
            .when(self.row.presence == Presence::Open, move |d| d.bg(idle))
            .child(self.glyph(palette))
            .child(label(
                &self.row,
                self.input.as_ref(),
                (theme.mono_family(), palette),
            ))
            .children(buttons);
        let shell = self.drag.style(shell, palette);
        let shell = tracked(shell, &self.row.id, &self.bounds);
        let payload = drag_payload::of(&self.row);
        let ghost = ghost_of(self.row.clone(), self.expansion, self.width);
        let handler = Rc::clone(&self.handler);
        let shell = shell.when(live, move |d| draggable(d, payload, ghost, &handler));
        let shell = with_menu(shell, &self.row.id, &self.handler);
        let (id, handler) = (self.row.id.clone(), Rc::clone(&self.handler));
        shell.when(live, move |d| open_on_click(d, id, handler))
    }
}

impl TreeRow {
    fn glyph(
        &self,
        palette: &Palette,
    ) -> gpui::AnyElement {
        let editing = self.icon_editing.clone();
        glyph_box((&self.row, self.expansion), editing, palette)
    }
}

fn buttons_shown(input: Option<Trailing>) -> bool {
    input.is_none_or(|trailing| trailing == Trailing::Kept)
}

fn ghost_of(
    row: Row,
    expansion: Expansion,
    width: f32,
) -> GhostBuild {
    Rc::new(move |_| ghost_row(&row, expansion, width).into_any_element())
}

fn ghost_row(
    row: &Row,
    expansion: Expansion,
    width: f32,
) -> TreeRow {
    TreeRow {
        row: row.clone(),
        expansion,
        input: None,
        handler: Rc::new(|_, _, _| {}),
        drag: RowDrag::lifted(width),
        bounds: RowBounds::default(),
        width,
        icon_editing: None,
    }
}

fn menu_action(
    row: &RowId,
    at: gpui::Point<gpui::Pixels>,
) -> RowAction {
    RowAction::Menu {
        row: row.clone(),
        at,
    }
}

fn with_menu(
    shell: Stateful<gpui::Div>,
    row: &RowId,
    handler: &RowHandler,
) -> Stateful<gpui::Div> {
    let (row, handler) = (row.clone(), Rc::clone(handler));
    shell.on_mouse_down(MouseButton::Right, move |event, window, cx| {
        handler(menu_action(&row, event.position), window, cx);
    })
}

fn open_on_click(
    shell: Stateful<gpui::Div>,
    row: RowId,
    handler: RowHandler,
) -> Stateful<gpui::Div> {
    shell.on_click(move |event: &ClickEvent, window, cx| {
        let action = match event.click_count() {
            0 | 1 => RowAction::Open(row.clone()),
            _ => RowAction::Rename(row.clone()),
        };
        handler(action, window, cx);
    })
}

fn glyph_box(
    (row, expansion): (&Row, Expansion),
    icon_editing: Option<WeakEntity<Sidebar>>,
    palette: &Palette,
) -> gpui::AnyElement {
    let frame = div().flex().flex_none().items_center().justify_center();
    match RowGlyph::of(row, expansion) {
        RowGlyph::Spinner => frame
            .size(px(GLYPH))
            .child(
                FlickerSpinner::new(SharedString::from(format!("spin-{}", row.id.as_str())))
                    .size(px(SPINNER)),
            )
            .into_any_element(),
        RowGlyph::Mark => frame
            .size(px(HEADER_GLYPH))
            .children(header_mark(row, icon_editing, palette))
            .into_any_element(),
        RowGlyph::Icon(name, weight, tone) => {
            let icon = Icon::new(name).weight(weight).size(px(GLYPH));
            let icon = match tone.color() {
                Some(color) => icon.color(color),
                None => icon,
            };
            frame.size(px(GLYPH)).child(icon).into_any_element()
        },
    }
}

fn header_mark(
    row: &Row,
    icon_editing: Option<WeakEntity<Sidebar>>,
    palette: &Palette,
) -> Option<gpui::AnyElement> {
    let mark = mark_view(row.mark.as_ref()?, px(HEADER_GLYPH), palette);
    let Some(sidebar) = icon_editing else {
        return Some(mark);
    };
    Some(editable_mark::editable_mark(
        row,
        (mark, px(HEADER_GLYPH)),
        sidebar,
    ))
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, MouseButton, TestAppContext, point, px};

    use super::*;

    use crate::test_support::{chat, measure, mount, recording_handler};

    const BUTTONS: [(Option<Trailing>, bool); 3] = [
        (None, true),
        (Some(Trailing::Kept), true),
        (Some(Trailing::Hidden), false),
    ];

    #[test]
    fn a_rename_keeps_the_hover_buttons_and_a_branch_name_hides_them() {
        for (input, shown) in BUTTONS {
            assert_eq!(buttons_shown(input), shown, "{input:?}");
        }
    }

    #[gpui::test]
    fn the_ghost_is_the_bare_row_without_the_list_margins(cx: &mut TestAppContext) {
        let ghost = ghost_of(chat("Refactor"), Expansion::Expanded, 180.0);
        let bounds = measure(cx, move |_, cx| ghost(cx));
        assert_eq!(f32::from(bounds.size.height), 36.0);
    }

    #[gpui::test]
    fn the_ghost_under_the_pointer_takes_no_part_in_the_drag(cx: &mut TestAppContext) {
        let (handler, seen) = recording_handler();
        let cx = mount(cx, move |_, _| TreeRow {
            row: chat("Refactor"),
            expansion: Expansion::Expanded,
            input: None,
            drag: RowDrag::lifted(294.0),
            bounds: RowBounds::default(),
            width: 294.0,
            handler: Rc::clone(&handler),
            icon_editing: None,
        });
        let none = Modifiers::none();
        let start = point(px(40.0), px(18.0));
        cx.simulate_mouse_down(start, MouseButton::Left, none);
        cx.simulate_mouse_move(point(px(80.0), px(18.0)), MouseButton::Left, none);
        cx.simulate_mouse_move(point(px(120.0), px(18.0)), MouseButton::Left, none);
        cx.simulate_mouse_up(point(px(120.0), px(18.0)), MouseButton::Left, none);
        assert!(seen.borrow().is_empty(), "the ghost reports nothing");
    }

    #[gpui::test]
    fn a_row_is_the_spec_height_plus_its_vertical_margin(cx: &mut TestAppContext) {
        let (handler, _) = recording_handler();
        let bounds = measure(cx, move |_, _| TreeRow {
            row: chat("Refactor"),
            expansion: Expansion::Expanded,
            input: None,
            drag: RowDrag::default(),
            bounds: RowBounds::default(),
            width: 294.0,
            handler: Rc::clone(&handler),
            icon_editing: None,
        });
        assert_eq!(f32::from(bounds.size.height), 38.0);
    }
}
