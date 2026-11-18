//! Supervises the Crowbar daemon sidecar: finds, spawns, health-checks, recovers and stops it,
//! and reports its state as a stream of [`DaemonEvent`]s.

mod error;
mod health;
mod supervisor;
mod supervisor_config;
#[cfg(test)]
mod test_support;

pub use error::Error;
pub use health::{
    Health, MigrationProgress, Probe, fetch, parse_health, probe_ready, wait_for_health,
};
pub use supervisor::{DaemonEvent, DaemonLog, Supervisor};
pub use supervisor_config::{DaemonPaths, SupervisorConfig, Timings, child_home, locate_binary};
