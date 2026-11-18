//! The host registry: paired hosts, their endpoints and the hosts file.

mod adapters;
mod endpoint;
mod first_connected;
mod host_entry;
mod host_registry;
mod host_target;
mod hosts_document;
mod hosts_storage;
#[cfg(test)]
mod test_support;

pub use endpoint::Endpoint;
pub use host_registry::HostRegistry;
pub(crate) use host_target::{HostTarget, check_registrable};
