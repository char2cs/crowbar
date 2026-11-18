/// Whether something native (a menu, popover or drag ghost) is drawn over the panes.
///
/// Native views paint above GPUI pixels, so while an overlay is active the host hides its web
/// views. That is a shell-internal visibility change: no pane is told.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum Overlay {
    /// Nothing covers the panes.
    #[default]
    Inactive,
    /// Panes are hidden so the overlay can show.
    Active,
}
