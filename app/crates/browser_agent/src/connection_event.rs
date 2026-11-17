use browser_nav::AgentAnswer;

pub(crate) enum ConnectionEvent {
    Frame(String),
    Lost,
    Done(String, AgentAnswer),
    TabsChanged,
    Idle,
    WindowGone,
}
