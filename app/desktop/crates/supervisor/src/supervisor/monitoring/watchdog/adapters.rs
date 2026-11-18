//! Reads process CPU time from /proc for the watchdog.

mod proc_stat;

pub(crate) use proc_stat::proc_stat;
