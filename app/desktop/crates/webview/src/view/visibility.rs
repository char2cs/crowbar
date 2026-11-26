/// Whether a web view is drawn. Hiding keeps the page alive and sends no lifecycle event.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Visibility {
    /// Drawn above the window contents.
    Shown,
    /// Not drawn, still loaded.
    Hidden,
    /// Not seen: the shell draws over it, and it returns without the blank frame a hidden view
    /// shows when it is drawn again.
    Covered,
}
