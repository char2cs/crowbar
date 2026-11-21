//! What a session tells its host, apart from the frames it draws.

/// A turn in a session's life that the host acts on: it remembers the session, closes the tab
/// of one that ended unseen, or tells the user it could not start.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Report {
    /// The daemon made this session for the view; the host keeps the id to attach to it again.
    Bound(String),
    /// The saved session ended while nothing watched it; nothing replaces it.
    Gone,
    /// The session could not be created or attached, for this reason.
    Failed(String),
    /// The closed view's session could not be stopped, in the daemon's words.
    StopFailed(String),
}
