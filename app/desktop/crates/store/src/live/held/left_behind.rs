//! What a removed host's views showed, kept until the shell closes them.

#[derive(Debug, Default)]
pub(crate) struct LeftBehind {
    pub(crate) chats: Vec<String>,
    pub(crate) projects: Vec<String>,
}
