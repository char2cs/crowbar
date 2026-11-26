use crate::linux::float::Card;

/// What is cut out of a web view's window.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PageCut {
    /// A hole, in device pixels from the page's top-left corner, as the pane handle hangs into.
    Hole(Card),
    /// All of it: the page keeps drawing behind the shell and returns without a blank frame,
    /// which unmapping its window would show.
    Everything,
}
