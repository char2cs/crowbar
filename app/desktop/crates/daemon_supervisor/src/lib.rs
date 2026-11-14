//! Supervises the Crowbar daemon sidecar: finds, spawns, health-checks, recovers and stops it,
//! and reports its state as a stream of [`DaemonEvent`]s.

mod bus;
mod command;
mod crash_report;
mod daemon_event;
mod daemon_log;
mod daemon_paths;
mod error;
mod exoneration;
mod failure_tracker;
mod health;
mod locate_binary;
mod output_pump;
mod phase;
mod probe;
mod process_cpu;
mod restart_budget;
mod running;
mod spawn_daemon;
mod startup;
mod stop;
mod supervise;
mod supervision;
mod supervisor;
mod supervisor_config;
mod timings;
mod verdict;
mod watch_context;
mod watch_state;
mod watchdog;
mod wedge;

pub use daemon_event::DaemonEvent;
pub use daemon_log::DaemonLog;
pub use daemon_paths::{DaemonPaths, child_home};
pub use error::Error;
pub use health::{Health, parse_health, wait_for_health};
pub use locate_binary::locate_binary;
pub use probe::{Probe, probe_ready};
pub use supervisor::Supervisor;
pub use supervisor_config::SupervisorConfig;
pub use timings::Timings;
