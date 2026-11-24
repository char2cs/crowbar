use std::path::PathBuf;
use std::sync::Arc;

/// What the web engine reports about the page of a view.
#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum PageEvent {
    /// The page is about to navigate to the URL; the handler's answer decides whether it may.
    Navigating(String),
    /// A navigation to the URL started loading.
    Loading(String),
    /// The page at the URL finished loading.
    Loaded(String),
    /// The page set its title.
    Title(String),
    /// Files from the operating system were dropped on the page, at this point in logical pixels
    /// from the page's top left. The web view takes the drop, so the page gets no drop event of
    /// its own, as Tauri's web views take file drops.
    FilesDropped {
        /// The dropped files.
        paths: Vec<PathBuf>,
        /// Distance from the page's left edge.
        x: f64,
        /// Distance from the page's top edge.
        y: f64,
    },
    /// A mouse button went down on the page. Reported on Linux, where a press does not give the
    /// page the keyboard by itself; the page still gets its own pointer events.
    Pressed,
    /// The pointer moved over the page, to this point in logical pixels from the page's top left.
    /// The shell's own hover and edge triggers cannot see the pointer over a native page.
    /// PLATFORM: Linux only; GTK already compresses motion to the latest position per frame.
    PointerMoved {
        /// Distance from the page's left edge.
        x: f64,
        /// Distance from the page's top edge.
        y: f64,
    },
}

/// Receives every [`PageEvent`] of one web view on the platform thread. Its answer matters only
/// for [`PageEvent::Navigating`]: `false` cancels the navigation.
pub type PageHandler = Arc<dyn Fn(PageEvent) -> bool + Send + Sync + 'static>;
