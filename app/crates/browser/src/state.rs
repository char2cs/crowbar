//! Page state shown in the native chrome, folded from engine reports and agent answers.

mod agent_answer;
mod page_state;
mod page_update;
mod tab_name;

pub use agent_answer::AgentAnswer;
pub use page_state::PageState;
pub use page_update::PageUpdate;
pub use tab_name::tab_name;
