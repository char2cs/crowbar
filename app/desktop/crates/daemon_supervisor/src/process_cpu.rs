use std::time::Duration;

use tokio::process::Command;

use crate::wedge::{parse_proc_stat_cpu, parse_ps_cpu_time, proc_state_is_stopped};

async fn proc_stat(pid: u32) -> Option<String> {
    tokio::fs::read_to_string(format!("/proc/{pid}/stat"))
        .await
        .ok()
}

async fn ps(args: [&str; 3], pid: u32) -> Option<String> {
    let output = Command::new("ps")
        .args(args)
        .arg(pid.to_string())
        .output()
        .await
        .ok()?;
    output
        .status
        .success()
        .then(|| String::from_utf8_lossy(&output.stdout).into_owned())
}

pub(crate) async fn cpu_time(pid: u32) -> Option<Duration> {
    if let Some(stat) = proc_stat(pid).await {
        return parse_proc_stat_cpu(&stat, rustix::param::clock_ticks_per_second());
    }
    parse_ps_cpu_time(&ps(["-o", "time=", "-p"], pid).await?)
}

pub(crate) async fn is_suspended(pid: u32) -> bool {
    if let Some(stat) = proc_stat(pid).await {
        return proc_state_is_stopped(&stat);
    }
    ps(["-o", "stat=", "-p"], pid)
        .await
        .is_some_and(|state| state.trim_start().starts_with('T'))
}

#[cfg(test)]
#[path = "process_cpu_tests.rs"]
mod tests;
