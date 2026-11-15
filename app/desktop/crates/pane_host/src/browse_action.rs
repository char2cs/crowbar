use browser_nav::BrowserUrl;

/// What the shell asks of a browser pane's web view.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum BrowseAction {
    /// Load a page a browser pane may load.
    Go(BrowserUrl),
    /// One page back in the view's history.
    Back,
    /// One page forward in the view's history.
    Forward,
    /// Load the current page again.
    Reload,
}
