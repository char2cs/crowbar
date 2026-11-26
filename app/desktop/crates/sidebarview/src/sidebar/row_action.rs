//! What a row reports to the sidebar when the user acts on it.

mod drag_point;
mod drag_press;

pub(crate) use drag_point::DragPoint;
pub(crate) use drag_press::DragPress;

use std::rc::Rc;

use gpui::{App, Pixels, Point, Window};
use sidebar::{CopyNote, Intent, ProjectId, RowId};

#[derive(Debug, Clone, PartialEq)]
pub(crate) enum RowAction {
    Open(RowId),
    Rename(RowId),
    Menu {
        row: RowId,
        at: Point<Pixels>,
    },
    RepoMenu {
        row: RowId,
        at: Point<Pixels>,
    },
    FootMenu {
        project: ProjectId,
        at: Point<Pixels>,
    },
    Intent(Intent),
    Copied(String, CopyNote),
    Press(DragPress),
    DragOver(DragPoint),
    Dropped(RowId, RowId),
    Edge(f32),
}

pub(crate) type RowHandler = Rc<dyn Fn(RowAction, &mut Window, &mut App)>;
