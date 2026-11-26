//! Whether this computer's projects have loaded, and losing that snapshot on demand.

pub(super) mod projects_load;
#[cfg(feature = "fake")]
mod projects_loss;
