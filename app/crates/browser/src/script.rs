//! JavaScript injected into the page: agent operations, eval and element picking.

mod agent;
mod page_calls;
mod pick;

pub use agent::{agent_script, eval_script};
pub use pick::{cancel_pick_script, pick_script};
