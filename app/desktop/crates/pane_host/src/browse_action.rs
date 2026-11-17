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
    /// Start picking elements in the page; the result comes back as a page update.
    Pick,
    /// End a pick in progress.
    CancelPick,
    /// Run a script the shell wrote in the page.
    Script(String),
}
