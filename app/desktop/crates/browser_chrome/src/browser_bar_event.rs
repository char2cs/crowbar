/// What the user asked of the browser pane.
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum BrowserBarEvent {
    /// Go to what was typed in the address field, which is not yet a URL.
    Go(String),
    /// Go back one page.
    Back,
    /// Go forward one page.
    Forward,
    /// Load the current page again.
    Reload,
    /// Start picking an element in the page, or end the pick in progress.
    TogglePick,
}
