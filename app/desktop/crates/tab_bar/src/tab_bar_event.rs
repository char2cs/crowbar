use model::Command;

/// What the tab bar asks its owner to do.
#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum TabBarEvent {
    /// Apply these commands in order; each is one step of the model.
    Apply(Vec<Command>),
    /// Show or hide the sidebar.
    ToggleSidebar,
}
