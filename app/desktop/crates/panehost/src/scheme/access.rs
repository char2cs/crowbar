//! Who may load what over `crowbar://`: policy, authorized requests, callers, cross-host reads.

pub(super) mod authorized;
pub(super) mod cross_host;
pub(super) mod page_caller;
pub(super) mod policy;
