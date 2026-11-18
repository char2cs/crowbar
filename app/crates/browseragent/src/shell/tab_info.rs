use serde::Serialize;

/// One browser tab as the daemon's `browser_tabs` tool shows it.
#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct TabInfo {
    /// The tab id the tools take.
    pub id: String,
    /// The chat the tab belongs to.
    #[serde(rename = "chatId")]
    pub chat_id: String,
    /// The address of the page.
    pub url: String,
    /// The page title, empty until it has one.
    pub title: String,
    /// Whether a page is loading.
    pub loading: bool,
}
