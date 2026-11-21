use crate::machine::migration_step::MigrationStep;
use crate::machine::outage::Outage;

/// What the daemon supervisor tells the shell, in the shape the view needs. The app crate
/// converts each supervisor event into one of these; this crate stays independent of it.
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Input {
    /// The daemon is being spawned or has not answered its health check yet.
    Starting,
    /// The daemon is being started again, after it died or on the user's word.
    Restarted,
    /// The daemon migrates its data.
    Migrating {
        /// The step it is on; `None` before the first step.
        step: Option<MigrationStep>,
    },
    /// The daemon serves.
    Ready,
    /// The daemon halted on purpose; no restart fixes it.
    Halted {
        /// The machine-readable halt code, when given.
        reason: Option<String>,
        /// What the daemon said.
        message: String,
    },
    /// The daemon is not serving although it was started.
    Unavailable(Outage),
}
