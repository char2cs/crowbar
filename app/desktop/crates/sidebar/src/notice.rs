//! Why a space's chats did not load.

/// A load failure shown above the tree with a Retry button.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Notice {
    /// The whole space failed.
    Project,
    /// One repository failed.
    Repo(String),
}

impl Notice {
    /// The sentence shown.
    pub fn text(&self) -> String {
        match self {
            Self::Project => "Couldn't load this project's chats".into(),
            Self::Repo(name) => format!("Couldn't load chats in {name}"),
        }
    }
}

#[cfg(test)]
#[path = "notice_tests.rs"]
mod tests;
