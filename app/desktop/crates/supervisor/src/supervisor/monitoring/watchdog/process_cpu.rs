use std::time::Duration;

use tokio::process::Command;

use crate::supervisor::monitoring::watchdog::adapters::proc_stat;
use crate::supervisor::monitoring::watchdog::wedge::{
    parse_proc_stat_cpu, parse_ps_cpu_time, proc_state_is_stopped,
};

const PS: &str = "/bin/ps";
const PS_TIMEOUT: Duration = Duration::from_secs(2);

async fn ps(
    args: &[&str],
    pid: u32,
) -> Option<String> {
    let run = Command::new(PS)
        .args(args)
        .arg(pid.to_string())
        .kill_on_drop(true)
        .output();
    let finished = match tokio::time::timeout(PS_TIMEOUT, run).await {
        Ok(Ok(finished)) => finished,
        Ok(Err(e)) => {
            ps_failed(&e);
            return None;
        },
        Err(elapsed) => {
            ps_timed_out(&elapsed);
            return None;
        },
    };
    finished
        .status
        .success()
        .then(|| String::from_utf8_lossy(&finished.stdout).into_owned())
}

fn ps_failed(e: &std::io::Error) {
    tracing::debug!("{PS}: {e}");
}

fn ps_timed_out(elapsed: &tokio::time::error::Elapsed) {
    tracing::warn!("{PS} did not answer within {PS_TIMEOUT:?}: {elapsed}");
}

pub(crate) async fn cpu_time(pid: u32) -> Option<Duration> {
    if let Some(stat) = proc_stat(pid).await {
        return parse_proc_stat_cpu(&stat, rustix::param::clock_ticks_per_second());
    }
    parse_ps_cpu_time(&ps(&["-o", "time=", "-p"], pid).await?)
}

pub(crate) async fn is_suspended(pid: u32) -> bool {
    if let Some(stat) = proc_stat(pid).await {
        return proc_state_is_stopped(&stat);
    }
    ps(&["-o", "stat=", "-p"], pid)
        .await
        .is_some_and(|state| state.trim_start().starts_with('T'))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn a_running_process_has_cpu_time_and_is_not_suspended() {
        let pid = std::process::id();
        assert!(cpu_time(pid).await.is_some());
        assert!(!is_suspended(pid).await);
    }

    #[tokio::test]
    async fn a_process_that_does_not_exist_reads_as_unknown_and_running() {
        let gone = i32::MAX as u32;
        assert_eq!(cpu_time(gone).await, None);
        assert!(!is_suspended(gone).await);
    }
}
