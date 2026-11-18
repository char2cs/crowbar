//! Supervises the Crowbar daemon sidecar: finds, spawns, health-checks, recovers and stops it,
//! and reports its state as a stream of [`DaemonEvent`]s.

mod config;
mod error;
mod health;
mod supervisor;
#[cfg(test)]
mod test_support;

pub use config::{Config, DaemonPaths, Timings, child_home, locate_binary};
pub use error::Error;
pub use health::{
    Health, MigrationProgress, Probe, fetch, parse_health, probe_ready, wait_for_health,
};
pub use supervisor::{DaemonEvent, DaemonLog, Supervisor};
