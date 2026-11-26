#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum HaltReason {
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
    pub(crate) fn from_code(code: Option<&str>) -> Self {
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

    pub(crate) fn code(&self) -> Option<&str> {
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

    pub(crate) fn explanation(&self) -> &'static str {
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

    #[test]
    fn known_codes_round_trip_and_explain_and_others_are_kept_for_display() {
        const CASES: &[(Option<&str>, bool)] = &[
            (Some("layout_newer"), true),
            (Some("layout_unreadable"), true),
            (Some("ledger_corrupt"), true),
            (Some("io_failed"), true),
            (Some("plan_failed"), true),
            (Some("preflight_failed"), true),
            (Some("step_failed"), true),
            (Some("verify_failed"), true),
            (Some("no_implementation"), true),
            (Some("brand_new"), false),
            (None, false),
        ];
        let generic = HaltReason::Other(None).explanation();
        for &(code, known) in CASES {
            let reason = HaltReason::from_code(code);
            assert_eq!(reason.code(), code);
            assert_eq!(reason.explanation() != generic, known, "{code:?}");
        }
    }
}
