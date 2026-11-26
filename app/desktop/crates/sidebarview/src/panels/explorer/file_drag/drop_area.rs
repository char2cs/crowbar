//! The Files rows' area during a row drag: the root target away from any row, the edges that
//! scroll it and the drop.

use std::rc::Rc;

use gpui::{App, Context, Div, DragMoveEvent, InteractiveElement, Styled, Window};
use sidebar::EdgeScroll;
use ui::DraggedRow;

use crate::sidebar::{FileGesture, RowAction, RowHandler, Sidebar};

impl Sidebar {
    // SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/file-explorer/file-explorer/components/file-explorer-tree.tsx
    pub(crate) fn file_drop_area(
        &self,
        body: Div,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let handler = Self::row_handler(cx);
        let (moved, dropped) = (Rc::clone(&handler), handler);
        let area = body
            .on_drag_move(move |event: &DragMoveEvent<DraggedRow>, window, cx| {
                report_roam(&moved, event, window, cx);
            })
            .on_drop(move |_: &DraggedRow, window, cx| {
                dropped(RowAction::File(FileGesture::Dropped), window, cx);
            });
        if !self
            .file_drag
            .target()
            .is_some_and(|target| target.is_root())
        {
            return area;
        }
        let palette = ui::Theme::of(cx).palette(ui::Surface::Chrome);
        area.border_2()
            .border_dashed()
            .border_color(palette.secondary())
            .bg(palette.secondary())
    }
}

fn report_roam(
    handler: &RowHandler,
    event: &DragMoveEvent<DraggedRow>,
    window: &mut Window,
    cx: &mut App,
) {
    for gesture in roaming(event) {
        handler(RowAction::File(gesture), window, cx);
    }
}

fn roaming(event: &DragMoveEvent<DraggedRow>) -> [FileGesture; 2] {
    let at = event.event.position;
    if !event.bounds.contains(&at) {
        return [FileGesture::Away, FileGesture::Edge(0.0)];
    }
    let (top, bottom) = (event.bounds.top(), event.bounds.bottom());
    let pull = EdgeScroll::between((top.into(), bottom.into())).pull(at.y.into());
    [FileGesture::Roam(at), FileGesture::Edge(pull)]
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, MouseButton, Point};

    use crate::test_support::prelude::*;

    const FAR_BELOW_THE_ROWS: f32 = 700.0;

    fn drag_through(
        points: &[Point<gpui::Pixels>],
        cx: &mut gpui::VisualTestContext,
    ) {
        for at in points {
            cx.simulate_mouse_move(*at, MouseButton::Left, Modifiers::none());
        }
    }

    #[gpui::test]
    fn a_row_released_away_from_every_row_moves_to_the_root(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        sidebar.update_in(cx, |this, window, cx| {
            this.tree.select_file(Some("src/main.rs".into()));
            window.focus(&this.files_focus, cx);
        });
        cx.run_until_parked();
        let main = cx.debug_bounds("file-selected").expect("main.rs is drawn");
        let start = main.center();
        let below = point(start.x, px(FAR_BELOW_THE_ROWS));
        cx.simulate_mouse_down(start, MouseButton::Left, Modifiers::none());
        let lifting = [
            start + point(px(0.0), px(3.0)),
            start + point(px(0.0), px(8.0)),
        ];
        drag_through(&lifting, cx);
        drag_through(&[below, below + point(px(1.0), px(0.0))], cx);
        let root = sidebar.read_with(cx, |this, _| {
            this.file_drag.target().is_some_and(|t| t.is_root())
        });
        assert!(root, "the panel is the target");
        cx.simulate_mouse_up(below, MouseButton::Left, Modifiers::none());
        let moved = Command::MoveFile {
            path: "src/main.rs".into(),
            new_path: "main.rs".into(),
        };
        assert!(recorder.sent().contains(&moved), "{:?}", recorder.sent());
    }
}
