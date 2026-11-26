//! Spaces in the switcher: projects, the active space and space list ports, icons, focus and running agents.

mod active_space;
mod pane_focus;
mod project;
mod project_id;
mod requests;
mod space_facts;
mod space_icon;
mod space_list;

pub use active_space::ActiveSpace;
pub use pane_focus::PaneFocus;
pub use pane_focus::focus::Focus;
pub use pane_focus::running_agent::RunningAgent;
pub use project::Project;
pub use project_id::ProjectId;
pub use requests::Requests;
pub use requests::icon_target::IconTarget;
pub use space_facts::SpaceFacts;
pub use space_icon::SpaceIcon;
pub use space_list::SpaceList;
pub use space_list::scope_sync::ScopeSync;
