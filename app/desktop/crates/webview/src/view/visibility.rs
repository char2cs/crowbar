/// Whether a web view is drawn. Hiding keeps the page alive and sends no lifecycle event.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Visibility {
    /// Drawn above the window contents.
    Shown,
    /// Not drawn, still loaded.
    Hidden,
}
