use browser_agent::AgentCommand;
use webview::ParentWindow;

#[derive(Debug, Clone, Copy)]
pub(crate) struct AgentCall<'a> {
    pub(crate) command: &'a AgentCommand,
    pub(crate) parent: Option<ParentWindow>,
}
