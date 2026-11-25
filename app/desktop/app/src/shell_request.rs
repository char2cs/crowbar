use crate::panes::content::new_tab::new_tab_action;
use crate::panes::terminals::link_choice;
#[derive(Debug, Clone, PartialEq)]
pub(crate) enum ShellRequest {
    Sidebar(sidebar::Command),
    Startup(startup::Command),
    /// The settings modal closed; the keyboard goes back to what held it when it opened.
    CloseSettings(Option<gpui::FocusHandle>),
    /// A pane reported its unsaved work again.
    ResumeClosing,
    /// A `mod+shift+<digit>` press selected the project in this slot (1 to 9).
    SelectProject(u8),
    /// The user answered the unsaved changes question.
    Closing(crate::close_choice::CloseChoice),
    /// The user agreed to drop a pane's unsaved edits and read its file again.
    ReloadDiscarding(protocol::PaneId),
    DataChanged,
    Outcome(store::Outcome),
    Command(keymap::ShellCommand),
    /// A page forwarded a chord that applies only while its surface shows.
    Contextual(keymap::ContextualCommand),
    /// The stored keybinding overrides changed.
    KeymapChanged,
    /// A setting the panes follow changed.
    PaneSettingsChanged,
    /// The stored theme, theme mode or UI font size changed.
    AppearanceChanged,
    NewTabChoice {
        pane: protocol::PaneId,
        action: new_tab_action::NewTabAction,
    },
    /// The daemon asked for something of a browser tab.
    Agent(browseragent::AgentCommand),
    Browse {
        pane: protocol::PaneId,
        event: browserbar::Event,
    },
    /// A terminal pane told its host something.
    Terminal {
        pane: protocol::PaneId,
        event: terminalview::Event,
    },
    /// A terminal pane's session was bound, ended unseen, or could not start.
    TerminalSession {
        pane: protocol::PaneId,
        report: ptysession::Report,
    },
    /// The first-run screens asked something of the shell.
    FirstRun(oobe::Event),
    /// A file pane's document moved: read, edited, saved or refused.
    Document(crate::panes::editor::document_request::DocumentRequest),
    /// The user chose a row of a terminal URL's menu.
    Link {
        pane: protocol::PaneId,
        url: String,
        choice: link_choice::LinkChoice,
    },
}
