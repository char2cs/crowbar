//! Where the input-method composition is painted.

use terminal::CursorCell;

use crate::terminal_view::paint::cursor::look::Look;

#[derive(Clone, Copy)]
pub(crate) struct PreeditSite<'a> {
    pub(crate) cell: &'a CursorCell,
    pub(crate) look: &'a Look<'a>,
}
