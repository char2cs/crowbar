use crate::report::migration_step::MigrationStep;
use crate::report::outage::Outage;

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Input {
    Starting,
    Restarted,
    Migrating {
        step: Option<MigrationStep>,
    },
    Ready,
    Halted {
        reason: Option<String>,
        message: String,
    },
    Unavailable(Outage),
}
