//! Spaces in the switcher: projects, the active space and space list ports, icons, focus and running agents.

mod active_space;
mod focus;
mod icon_choice;
mod icon_target;
mod pane_focus;
mod project;
mod project_id;
mod requests;
mod running_agent;
mod scope_sync;
mod space_facts;
mod space_icon;
mod space_list;

pub use active_space::ActiveSpace;
pub use focus::Focus;
pub use icon_target::IconTarget;
pub use pane_focus::PaneFocus;
pub use project::Project;
pub use project_id::ProjectId;
pub use requests::Requests;
pub use running_agent::RunningAgent;
pub use scope_sync::ScopeSync;
pub use space_facts::SpaceFacts;
pub use space_icon::SpaceIcon;
pub use space_list::SpaceList;
