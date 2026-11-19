use std::path::PathBuf;
use std::sync::Arc;

/// What the web engine reports about the page of a view.
#[derive(Debug, Clone, PartialEq, Eq)]
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
    /// Files from the operating system were dropped on the page; the page still gets its own
    /// drop events.
    FilesDropped(Vec<PathBuf>),
    /// A mouse button went down on the page. Reported on Linux, where a press does not give the
    /// page the keyboard by itself; the page still gets its own pointer events.
    Pressed,
}

/// Receives every [`PageEvent`] of one web view on the platform thread. Its answer matters only
/// for [`PageEvent::Navigating`]: `false` cancels the navigation.
pub type PageHandler = Arc<dyn Fn(PageEvent) -> bool + Send + Sync + 'static>;
