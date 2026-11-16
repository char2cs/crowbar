//! A normal row of the chats tree.

use std::rc::Rc;
use std::sync::Arc;

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ClickEvent, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, SharedString, Stateful, StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{DraggedRow, Icon, Palette, Spinner, Surface};

use crate::cluster_spec::ClusterSpec;
use crate::expansion::Expansion;
use crate::mark_view::mark_view;
use crate::presence::Presence;
use crate::row::Row;
use crate::row_action::{RowAction, RowHandler};
use crate::row_bounds::RowBounds;
use crate::row_cluster::cluster;
use crate::row_drag::{RowDrag, tracked};
use crate::row_draggable::draggable;
use crate::row_focus::{ringed, row_focus};
use crate::row_ghost::GhostBuild;
use crate::row_glyph::RowGlyph;
use crate::row_id::RowId;
use crate::row_input::RowInput;
use crate::row_kind::RowKind;
use crate::row_label::label;
use crate::row_shell::row_shell;

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
        let shell = ringed(row_shell(id, self.row.depth, palette), &focus, palette)
            .group(group)
            .hover(move |style| style.bg(idle))
            .when(self.row.presence == Presence::Open, move |d| d.bg(idle))
            .child(glyph_box(&self.row, self.expansion, palette))
            .child(label(
                &self.row,
                self.input.as_ref(),
                (theme.mono_family(), palette),
            ))
            .child(cluster(&spec));
        let shell = self.drag.style(shell, palette);
        let shell = tracked(shell, &self.row.id, &self.bounds);
        let payload = payload_of(&self.row);
        let ghost = ghost_of(self.row.clone(), self.expansion, self.width);
        let (handler, free) = (Rc::clone(&self.handler), self.input.is_none());
        let shell = shell.when(free, move |d| draggable(d, payload, ghost, &handler));
        let shell = with_menu(shell, &self.row.id, &self.handler);
        let (id, handler) = (self.row.id.clone(), Rc::clone(&self.handler));
        shell.when(self.input.is_none(), move |d| open_on_click(d, id, handler))
    }
}

fn payload_of(row: &Row) -> DraggedRow {
    let id: Arc<str> = Arc::from(row.id.as_str());
    DraggedRow {
        chat: (row.kind == RowKind::Chat).then(|| Arc::clone(&id)),
        row: id,
    }
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
    row: &Row,
    expansion: Expansion,
    palette: &Palette,
) -> gpui::AnyElement {
    let frame = div().flex().flex_none().items_center().justify_center();
    match RowGlyph::of(row, expansion) {
        RowGlyph::Spinner => frame
            .size(px(GLYPH))
            .child(
                Spinner::new(SharedString::from(format!("spin-{}", row.id.as_str())))
                    .size(px(SPINNER)),
            )
            .into_any_element(),
        RowGlyph::Mark => frame
            .size(px(HEADER_GLYPH))
            .children(
                row.mark
                    .as_ref()
                    .map(|mark| mark_view(mark, px(HEADER_GLYPH), palette)),
            )
            .into_any_element(),
        RowGlyph::Icon(name, weight) => frame
            .size(px(GLYPH))
            .child(Icon::new(name).weight(weight).size(px(GLYPH)))
            .into_any_element(),
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::{chat, measure, recording_handler};

    #[test]
    fn only_a_chat_row_carries_a_chat_to_open() {
        let chat_row = payload_of(&chat("Refactor"));
        assert_eq!(
            chat_row.chat.as_deref(),
            Some(chat_row.row.as_ref()),
            "chat"
        );
        let folder = Row::new("f", RowKind::Folder, 1, "src");
        assert_eq!(payload_of(&folder).chat, None, "folder");
    }

    #[gpui::test]
    fn the_ghost_is_the_bare_row_without_the_list_margins(cx: &mut TestAppContext) {
        let ghost = ghost_of(chat("Refactor"), Expansion::Expanded, 180.0);
        let bounds = measure(cx, move |_, cx| ghost(cx));
        assert_eq!(f32::from(bounds.size.height), 36.0);
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
        });
        assert_eq!(f32::from(bounds.size.height), 38.0);
    }
}
