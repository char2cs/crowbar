//! What a row reports to the sidebar when the user acts on it.

use std::rc::Rc;

use gpui::{App, Pixels, Point, Window};

use crate::intent::Intent;
use crate::project_id::ProjectId;
use crate::row_id::RowId;

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
}

pub(crate) type RowHandler = Rc<dyn Fn(RowAction, &mut Window, &mut App)>;
