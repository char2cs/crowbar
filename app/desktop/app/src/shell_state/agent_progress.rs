use browser::AgentAnswer;
#[derive(Debug, PartialEq)]
pub(super) enum AgentProgress {
    Answered(AgentAnswer),
    Waiting,
}
