//! The app's open-file limit. macOS starts a GUI app at 256 and Rust, unlike Go, never
//! raises it — yet every frontend request and WebSocket is a unix socket of this process,
//! and at the ceiling the health watchdog mistakes `EMFILE` for a dead daemon.

/// The soft limit to move to, or `None` if the current one is as generous. macOS reports
/// an unlimited hard limit but refuses more than its per-process ceiling, so that binds
/// there; elsewhere `max_per_proc` is `u64::MAX` and the hard limit binds.
pub fn target_soft_limit(soft: u64, hard: u64, max_per_proc: u64) -> Option<u64> {
    let target = hard.min(max_per_proc);
    (target > soft).then_some(target)
}

/// The kernel's per-process descriptor ceiling, or `u64::MAX` where there is none
/// beyond the hard limit.
#[cfg(target_os = "macos")]
fn max_files_per_proc() -> u64 {
    /// Fallback when the sysctl cannot be read; the stock macOS value.
    const DEFAULT: u64 = 61_440;

    let name = c"kern.maxfilesperproc";
    let mut value: libc::c_int = 0;
    let mut size = std::mem::size_of::<libc::c_int>();
    // SAFETY: `name` is NUL-terminated and `value`/`size` are a matching out-param
    // pair for an integer sysctl.
    let rc = unsafe {
        libc::sysctlbyname(
            name.as_ptr(),
            std::ptr::addr_of_mut!(value).cast::<libc::c_void>(),
            &mut size,
            std::ptr::null_mut(),
            0,
        )
    };
    if rc == 0 && value > 0 {
        value as u64
    } else {
        DEFAULT
    }
}

#[cfg(all(unix, not(target_os = "macos")))]
fn max_files_per_proc() -> u64 {
    u64::MAX
}

/// What [`raise`] did, carried out to be reported once a logger exists: the raise
/// precedes the log plugin, and the number is the one that explains an `EMFILE`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Outcome {
    Raised { from: u64, to: u64 },
    AlreadyGenerous(u64),
    Failed(String),
}

impl std::fmt::Display for Outcome {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Raised { from, to } => {
                write!(f, "raised the open-file soft limit from {from} to {to}")
            }
            Self::AlreadyGenerous(soft) => write!(f, "open-file soft limit is already {soft}"),
            Self::Failed(why) => write!(f, "could not raise the open-file limit: {why}"),
        }
    }
}

/// Raises the soft open-file limit as far as the kernel allows. Best-effort: a failure
/// is reported, never fatal — the app just stays on the old ceiling.
#[cfg(unix)]
pub fn raise() -> Outcome {
    let mut lim = libc::rlimit {
        rlim_cur: 0,
        rlim_max: 0,
    };
    // SAFETY: `lim` is a valid, fully-initialised out-param.
    if unsafe { libc::getrlimit(libc::RLIMIT_NOFILE, &mut lim) } != 0 {
        return Outcome::Failed(format!(
            "reading it failed: {}",
            std::io::Error::last_os_error()
        ));
    }

    let soft = lim.rlim_cur;
    let Some(target) = target_soft_limit(soft, lim.rlim_max, max_files_per_proc()) else {
        return Outcome::AlreadyGenerous(soft);
    };

    lim.rlim_cur = target;
    // SAFETY: `lim` came from getrlimit; only its soft field changed, to a value
    // the kernel accepts (<= the hard limit and <= the per-process ceiling).
    if unsafe { libc::setrlimit(libc::RLIMIT_NOFILE, &lim) } != 0 {
        return Outcome::Failed(format!(
            "{soft} -> {target} refused: {}",
            std::io::Error::last_os_error()
        ));
    }
    Outcome::Raised {
        from: soft,
        to: target,
    }
}

#[cfg(not(unix))]
pub fn raise() -> Outcome {
    Outcome::AlreadyGenerous(0)
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The case the app actually ships in: launchd's 256, an "unlimited" hard
    /// limit, and a kernel that will not in fact go past kern.maxfilesperproc.
    #[test]
    fn claims_the_per_process_ceiling_when_the_hard_limit_is_unlimited() {
        assert_eq!(
            target_soft_limit(256, u64::MAX, 61_440),
            Some(61_440),
            "a GUI app inherits 256 and must climb to the kernel's real ceiling"
        );
    }

    #[test]
    fn never_asks_for_more_than_the_hard_limit() {
        assert_eq!(
            target_soft_limit(256, 1024, 61_440),
            Some(1024),
            "a hard limit below the kernel's ceiling is the binding one"
        );
    }

    #[test]
    fn leaves_an_already_generous_limit_alone() {
        assert_eq!(target_soft_limit(61_440, u64::MAX, 61_440), None);
        assert_eq!(
            target_soft_limit(70_000, u64::MAX, 61_440),
            None,
            "a soft limit above the ceiling is someone else's deliberate choice"
        );
    }

    /// The raise must actually take effect in this process — the whole point is
    /// that a Rust binary does not get one for free the way the Go daemon does.
    /// Run this under `ulimit -Sn 256` to exercise the launchd condition itself.
    #[cfg(unix)]
    #[test]
    fn raise_lifts_this_process_clear_of_the_default_ceiling() {
        let outcome = raise();
        eprintln!("fdlimit: {outcome}");
        assert!(
            !matches!(outcome, Outcome::Failed(_)),
            "the raise must not fail: {outcome}"
        );

        let mut lim = libc::rlimit {
            rlim_cur: 0,
            rlim_max: 0,
        };
        assert_eq!(unsafe { libc::getrlimit(libc::RLIMIT_NOFILE, &mut lim) }, 0);
        assert!(
            lim.rlim_cur > 256,
            "soft limit is still {}; the app would hit EMFILE under ordinary use \
             and its watchdog would blame the daemon for it",
            lim.rlim_cur
        );
    }
}
