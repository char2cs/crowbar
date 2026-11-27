use crate::panes::content::new_tab::new_tab_action;
use crate::panes::terminals::link_choice;
#[derive(Debug, Clone, PartialEq)]
pub(crate) enum ShellRequest {
    Sidebar(sidebar::Command),
    Startup(startup::Command),
    CloseSettings(Option<gpui::FocusHandle>),
    ResumeClosing,
    SelectProject(u8),
    Closing(crate::shell::dialogs::unsaved::close_choice::CloseChoice),
    ReloadDiscarding(protocol::PaneId),
    DataChanged,
    Outcome(store::Outcome),
    Command(keymap::ShellCommand),
    Contextual(keymap::ContextualCommand),
    KeymapChanged,
    PaneSettingsChanged,
    AppearanceChanged,
    NewTabChoice {
        pane: protocol::PaneId,
        action: new_tab_action::NewTabAction,
    },
    Agent(browseragent::AgentCommand),
    Browse {
        pane: protocol::PaneId,
        event: browserbar::Event,
    },
    Terminal {
        pane: protocol::PaneId,
        event: terminalview::Event,
    },
    TerminalSession {
        pane: protocol::PaneId,
        report: ptysession::Report,
    },
    FirstRun(oobe::Event),
    Document(crate::panes::editor::document_request::DocumentRequest),
    Link {
        pane: protocol::PaneId,
        url: String,
        choice: link_choice::LinkChoice,
    },
}
