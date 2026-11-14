//! The set of placements a drop policy allows.

use crate::drop_mode::DropMode;
use crate::folder_target::FolderTarget;

const BEFORE: u8 = 1;
const AFTER: u8 = 2;
const INTO: u8 = 4;
const FOLDER_BAND: f32 = 0.2;
const OTHER_BAND: f32 = 0.3;

/// A subset of before, after and into.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct DropModes(u8);

impl DropModes {
    /// Nothing is allowed; no indicator shows.
    pub const NONE: Self = Self(0);
    /// Before and after, no nesting.
    pub const REORDER: Self = Self(BEFORE | AFTER);
    /// Only nesting.
    pub const INTO: Self = Self(INTO);
    /// Everything.
    pub const ALL: Self = Self(BEFORE | AFTER | INTO);

    fn bit(mode: DropMode) -> u8 {
        match mode {
            DropMode::Before => BEFORE,
            DropMode::After => AFTER,
            DropMode::Into => INTO,
        }
    }

    /// Whether `mode` is allowed.
    pub fn contains(self, mode: DropMode) -> bool {
        self.0 & Self::bit(mode) != 0
    }

    /// The mode the pointer's height inside the target row selects. `ratio` is the pointer's
    /// distance from the row top over the row height; `folder` widens the middle band. A mode
    /// the policy refuses yields no drop.
    pub fn at(self, ratio: f32, folder: FolderTarget) -> Option<DropMode> {
        if self == Self::NONE {
            return None;
        }
        let band = match folder {
            FolderTarget::Yes => FOLDER_BAND,
            FolderTarget::No => OTHER_BAND,
        };
        let mode = if !self.contains(DropMode::Into) {
            Self::half(ratio)
        } else if self == Self::INTO {
            DropMode::Into
        } else if ratio < band {
            DropMode::Before
        } else if ratio > 1.0 - band {
            DropMode::After
        } else {
            DropMode::Into
        };
        self.contains(mode).then_some(mode)
    }

    fn half(ratio: f32) -> DropMode {
        if ratio < 0.5 {
            DropMode::Before
        } else {
            DropMode::After
        }
    }
}

#[cfg(test)]
#[path = "drop_modes_tests.rs"]
mod tests;
