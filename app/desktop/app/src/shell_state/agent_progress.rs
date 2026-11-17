use browser_nav::AgentAnswer;
#[derive(Debug, PartialEq)]
pub(super) enum AgentProgress {
    Answered(AgentAnswer),
    Waiting,
}
