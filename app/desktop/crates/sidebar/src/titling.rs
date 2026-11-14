//! Whether a chat title is final.

/// Provisional titles are drawn in italics until the agent names the chat.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Titling {
    /// A settled title.
    #[default]
    Final,
    /// A placeholder title.
    Provisional,
}
