/// Why the daemon refused to start, from its machine-readable halt code.
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum HaltReason {
    /// The data was written by a newer Crowbar.
    LayoutNewer,
    /// The data layout cannot be read.
    LayoutUnreadable,
    /// The migration ledger is damaged.
    LedgerCorrupt,
    /// Reading or writing the data failed.
    IoFailed,
    /// The migration plan could not be made.
    PlanFailed,
    /// A check before migrating failed.
    PreflightFailed,
    /// A migration step failed.
    StepFailed,
    /// The migrated data did not verify.
    VerifyFailed,
    /// A step has no implementation in this version.
    NoImplementation,
    /// A code this version does not know, or none at all.
    Other(Option<String>),
}

impl HaltReason {
    /// Maps the daemon's halt code.
    pub fn from_code(code: Option<&str>) -> Self {
        match code {
            Some("layout_newer") => Self::LayoutNewer,
            Some("layout_unreadable") => Self::LayoutUnreadable,
            Some("ledger_corrupt") => Self::LedgerCorrupt,
            Some("io_failed") => Self::IoFailed,
            Some("plan_failed") => Self::PlanFailed,
            Some("preflight_failed") => Self::PreflightFailed,
            Some("step_failed") => Self::StepFailed,
            Some("verify_failed") => Self::VerifyFailed,
            Some("no_implementation") => Self::NoImplementation,
            other => Self::Other(other.map(str::to_owned)),
        }
    }

    /// The machine-readable code, when the daemon gave one.
    pub fn code(&self) -> Option<&str> {
        match self {
            Self::LayoutNewer => Some("layout_newer"),
            Self::LayoutUnreadable => Some("layout_unreadable"),
            Self::LedgerCorrupt => Some("ledger_corrupt"),
            Self::IoFailed => Some("io_failed"),
            Self::PlanFailed => Some("plan_failed"),
            Self::PreflightFailed => Some("preflight_failed"),
            Self::StepFailed => Some("step_failed"),
            Self::VerifyFailed => Some("verify_failed"),
            Self::NoImplementation => Some("no_implementation"),
            Self::Other(code) => code.as_deref(),
        }
    }

    /// What happened and what to do about it, in a sentence or two.
    pub fn explanation(&self) -> &'static str {
        match self {
            Self::LayoutNewer => {
                "Your data was written by a newer version of Crowbar. Install the latest version to open it."
            },
            Self::LayoutUnreadable => {
                "Crowbar cannot read the layout of your data folder. Restore it from a backup."
            },
            Self::LedgerCorrupt => {
                "The record of finished upgrades is damaged. Restore your data from a backup."
            },
            Self::IoFailed => {
                "Crowbar could not read or write your data folder. Check its permissions and free disk space."
            },
            Self::PlanFailed => "Crowbar could not work out how to upgrade your data.",
            Self::PreflightFailed => {
                "A check before upgrading your data failed, so nothing was changed."
            },
            Self::StepFailed => {
                "An upgrade step failed. Your data was backed up before it started."
            },
            Self::VerifyFailed => {
                "The upgraded data did not pass its check. Your data was backed up before it started."
            },
            Self::NoImplementation => {
                "This version of Crowbar cannot perform a step your data needs. Install the latest version."
            },
            Self::Other(_) => "Crowbar's background service stopped before it finished starting.",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CODES: &[&str] = &[
        "layout_newer",
        "layout_unreadable",
        "ledger_corrupt",
        "io_failed",
        "plan_failed",
        "preflight_failed",
        "step_failed",
        "verify_failed",
        "no_implementation",
    ];

    #[test]
    fn every_known_code_round_trips_and_explains() {
        for &code in CODES {
            let reason = HaltReason::from_code(Some(code));
            assert!(!matches!(reason, HaltReason::Other(_)), "{code}");
            assert_eq!(reason.code(), Some(code));
            assert!(reason.explanation().len() > 20, "{code}");
        }
    }

    #[test]
    fn an_unknown_or_missing_code_is_kept_for_display() {
        assert_eq!(
            HaltReason::from_code(Some("brand_new")).code(),
            Some("brand_new")
        );
        assert_eq!(HaltReason::from_code(None).code(), None);
        assert!(matches!(
            HaltReason::from_code(None),
            HaltReason::Other(None)
        ));
    }
}
