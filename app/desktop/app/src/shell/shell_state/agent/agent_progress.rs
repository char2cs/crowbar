use browser::AgentAnswer;
#[derive(Debug, PartialEq)]
pub(crate) enum AgentProgress {
    Answered(AgentAnswer),
    Waiting,
}
