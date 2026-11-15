//! Which way a page scroll goes.

#[derive(Debug, Clone, Copy)]
pub(crate) enum PageDirection {
    Up,
    Down,
}
