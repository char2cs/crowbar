use crate::linux::float::Card;
use crate::linux::page_shape::rounded::Rounded;

/// What is cut out of a web view's window.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PageCut {
    /// A hole the pane handle hangs into, from the page's top-left corner in device pixels,
    /// and the corners the page rounds.
    Trimmed {
        /// The handle's hole.
        hole: Option<Card>,
        /// The rounding.
        round: Option<Rounded>,
    },
    /// All of it: the page keeps drawing behind the shell and returns without a blank frame,
    /// which unmapping its window would show.
    Everything,
}
