use serde::{Deserialize, Serialize};

/// What the app as a whole is doing, which decides what the shell renders.
///
/// While [`AppPhase::Migrating`] the shell shows a full-screen native migration view instead
/// of the tab layout.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(
    tag = "phase",
    rename_all = "camelCase",
    rename_all_fields = "camelCase"
)]
pub enum AppPhase {
    /// The daemon is being started or contacted.
    Starting,
    /// The daemon is migrating its data.
    Migrating {
        /// Short name of the current migration step.
        state: String,
        /// Overall progress in `0.0..=1.0`, if known.
        #[serde(skip_serializing_if = "Option::is_none")]
        progress: Option<f32>,
    },
    /// The daemon is ready and the layout is shown.
    Ready,
    /// The daemon cannot be reached.
    DaemonUnavailable {
        /// Why, for display.
        reason: String,
    },
}

impl AppPhase {
    /// Whether the tab layout may be shown.
    #[must_use]
    pub fn shows_layout(&self) -> bool {
        matches!(self, Self::Ready)
    }
}

#[cfg(test)]
#[path = "app_phase_tests.rs"]
mod tests;
