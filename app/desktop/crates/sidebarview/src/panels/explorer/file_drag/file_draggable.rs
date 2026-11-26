//! Makes a Files row pressable and draggable, and reports the pointer coming over it and
//! leaving it; each row clears only the target it set, as Zed's project panel does.

use std::rc::Rc;
use std::sync::Arc;

use gpui::{
    Div, DragMoveEvent, InteractiveElement, MouseButton, SharedString, Stateful,
    StatefulInteractiveElement,
};
use sidebar::{DropSpot, FileKind, FileMove};
use ui::DraggedRow;

use crate::panels::explorer::file_drag::file_ghost::file_ghost;
use crate::panels::explorer::file_tree_entry::FileTreeEntry;
use crate::sidebar::{FileGesture, RowAction, RowHandler};

pub(crate) fn file_draggable(
    row: Stateful<Div>,
    entry: &FileTreeEntry,
    handler: &RowHandler,
) -> Stateful<Div> {
    let dragged = FileMove::new(&entry.path, entry.kind);
    let spot = DropSpot::Row {
        path: entry.path.clone(),
        kind: entry.kind,
    };
    let (pressed, over) = (Rc::clone(handler), Rc::clone(handler));
    let name = SharedString::from(entry.name.clone());
    row.on_mouse_down(MouseButton::Left, move |event, window, cx| {
        let press = FileGesture::Press(dragged.clone(), event.position);
        pressed(RowAction::File(press), window, cx);
    })
    .on_drag(payload(entry), move |_, grabbed, _, cx| {
        file_ghost(name.clone(), grabbed, cx)
    })
    .on_drag_move(move |event: &DragMoveEvent<DraggedRow>, window, cx| {
        over(RowAction::File(gesture_over(&spot, event)), window, cx);
    })
}

fn payload(entry: &FileTreeEntry) -> DraggedRow {
    let path: Arc<str> = Arc::from(entry.path.as_str());
    let opens = entry.kind == FileKind::File;
    DraggedRow {
        row: Arc::clone(&path),
        chat: None,
        file: opens.then_some(path),
    }
}

fn gesture_over(
    spot: &DropSpot,
    event: &DragMoveEvent<DraggedRow>,
) -> FileGesture {
    let at = event.event.position;
    if !event.bounds.contains(&at) {
        return FileGesture::Left(spot.clone());
    }
    FileGesture::Over(spot.clone(), at)
}

#[cfg(test)]
mod tests {
    use gpui::{IntoElement, Modifiers, ParentElement, Styled, TestAppContext, div, point, px};
    use sidebar::{Expansion, GitStatus};

    use super::*;
    use crate::test_support::{mount, recording_handler};

    fn entry(
        path: &str,
        kind: FileKind,
    ) -> FileTreeEntry {
        FileTreeEntry {
            path: path.into(),
            name: path.into(),
            depth: 0,
            kind,
            status: GitStatus::Clean,
            counts: None,
        }
    }

    #[test]
    fn only_a_file_row_carries_a_file_to_open() {
        let folder = FileKind::Directory(Expansion::Collapsed);
        let cases = [(FileKind::File, Some("a.rs")), (folder, None)];
        for (kind, want) in cases {
            let dragged = payload(&entry("a.rs", kind));
            assert_eq!(dragged.file.as_deref(), want, "{kind:?}");
            assert_eq!(dragged.chat, None);
        }
    }

    #[gpui::test]
    fn dragging_a_row_reports_the_press_then_the_pointer_over_it_then_leaving(
        cx: &mut TestAppContext
    ) {
        let (handler, seen) = recording_handler();
        let cx = mount(cx, move |_, _| {
            let row = div().id("row").h(px(26.0)).w(px(200.0)).child("a.rs");
            let row = file_draggable(row, &entry("a.rs", FileKind::File), &handler);
            div().h(px(100.0)).child(row).into_any_element()
        });
        let (start, inside, below) = (
            point(px(20.0), px(10.0)),
            point(px(40.0), px(12.0)),
            point(px(40.0), px(80.0)),
        );
        cx.simulate_mouse_down(start, MouseButton::Left, Modifiers::none());
        let lifting = point(px(30.0), px(10.0));
        cx.simulate_mouse_move(lifting, MouseButton::Left, Modifiers::none());
        cx.simulate_mouse_move(inside, MouseButton::Left, Modifiers::none());
        cx.simulate_mouse_move(below, MouseButton::Left, Modifiers::none());
        let spot = DropSpot::Row {
            path: "a.rs".into(),
            kind: FileKind::File,
        };
        let files: Vec<FileGesture> = seen
            .borrow()
            .iter()
            .filter_map(|action| match action {
                RowAction::File(gesture) => Some(gesture.clone()),
                _ => None,
            })
            .collect();
        let want = [
            FileGesture::Press(FileMove::new("a.rs", FileKind::File), start),
            FileGesture::Over(spot.clone(), inside),
            FileGesture::Left(spot),
        ];
        assert_eq!(files, want);
    }
}
