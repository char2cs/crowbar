use crate::shell::chats::RowId;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RunningAgent {
    pub row: RowId,
    pub title: String,
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
