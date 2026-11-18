//! The host registry: paired hosts, their endpoints and the hosts file.

mod adapters;
mod document;
mod endpoint;
mod entry;
mod first_connected;
mod host_registry;
mod storage;
mod target;
#[cfg(test)]
mod test_support;

pub use endpoint::Endpoint;
pub use host_registry::HostRegistry;
pub(crate) use target::{Target, check_registrable};
