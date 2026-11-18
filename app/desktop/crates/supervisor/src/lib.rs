//! Supervises the Crowbar daemon sidecar: finds, spawns, health-checks, recovers and stops it,
//! and reports its state as a stream of [`DaemonEvent`]s.

mod config;
mod error;
mod health;
mod supervisor;
#[cfg(test)]
mod test_support;

pub use config::{Config, DaemonPaths, Timings, locate_binary};
pub use error::Error;
pub use health::{MigrationProgress, fetch};
pub use supervisor::{DaemonEvent, Supervisor};
