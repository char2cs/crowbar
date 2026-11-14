use gpui::SharedString;
use protocol::PaneId;

/// One chat of a tab as the strip shows it.
#[derive(Debug, Clone, PartialEq)]
pub struct MemberView {
    /// The pane that holds the chat.
    pub pane: PaneId,
    /// The chat's title.
    pub label: SharedString,
    /// Whether the title is still provisional and so drawn in italics.
    pub provisional: bool,
    /// Whether an agent turn or workspace operation is running; the glyph turns into a spinner.
    pub working: bool,
}
