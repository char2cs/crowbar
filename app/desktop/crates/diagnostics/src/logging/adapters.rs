//! The log store on disk and the system clock.

pub(crate) mod disk_log_store;
pub(crate) mod system_clock;

pub(crate) use disk_log_store::DiskLogStore;
