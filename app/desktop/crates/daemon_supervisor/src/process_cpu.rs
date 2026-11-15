use std::time::Duration;

use tokio::process::Command;

use crate::wedge::{parse_proc_stat_cpu, parse_ps_cpu_time, proc_state_is_stopped};

const PS: &str = "/bin/ps";
const PS_TIMEOUT: Duration = Duration::from_secs(2);

async fn proc_stat(pid: u32) -> Option<String> {
    tokio::fs::read_to_string(format!("/proc/{pid}/stat"))
        .await
        .map_err(|e| tracing::trace!("/proc/{pid}/stat: {e}"))
        .ok()
}

async fn ps(
    args: [&str; 3],
    pid: u32,
) -> Option<String> {
    let run = Command::new(PS)
        .args(args)
        .arg(pid.to_string())
        .kill_on_drop(true)
        .output();
    let output = match tokio::time::timeout(PS_TIMEOUT, run).await {
        Ok(Ok(output)) => output,
        Ok(Err(e)) => {
            tracing::debug!("{PS}: {e}");
            return None;
        },
        Err(_) => {
            tracing::warn!("{PS} did not answer within {PS_TIMEOUT:?}");
            return None;
        },
    };
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
