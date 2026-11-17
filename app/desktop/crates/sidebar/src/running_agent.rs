//! A chat whose agent is running now.

use crate::row_id::RowId;

/// An agent that can be killed from the sidebar.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RunningAgent {
    /// The chat's row.
    pub row: RowId,
    /// The chat's title, for the confirmation "Kill the agent on `<title>`?".
    pub title: String,
    /// How many clients are attached to it, this one included; 0 is an orphan.
    pub viewers: u32,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn an_agent_names_its_row_and_how_many_watch_it() {
        let agent = RunningAgent {
            row: RowId::from("c"),
            title: "t".into(),
            viewers: 0,
        };
        assert_eq!((agent.row.as_str(), agent.viewers), ("c", 0));
    }
}
