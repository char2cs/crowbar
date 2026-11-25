//! The crowbar:// router: the pane protocol, its policy, the grants a page reads another host's answers by, routes, authorization, origin table, assignments and status codes.

mod assignment;
mod authorized;
mod cross_host;
mod origin_table;
mod pane_protocol;
mod policy;
mod route;
mod status;

pub use assignment::Assignment;
pub use origin_table::OriginTable;
pub use pane_protocol::PaneProtocol;
