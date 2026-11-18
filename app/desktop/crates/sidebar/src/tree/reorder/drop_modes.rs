//! The set of placements a drop policy allows.

use crate::shell::DropMode;
use crate::tree::reorder::folder_target::FolderTarget;

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
    pub fn contains(
        self,
        mode: DropMode,
    ) -> bool {
        self.0 & Self::bit(mode) != 0
    }

    /// The mode the pointer's height inside the target row selects. `ratio` is the pointer's
    /// distance from the row top over the row height; `folder` widens the middle band. A mode
    /// the policy refuses yields no drop.
    pub fn at(
        self,
        ratio: f32,
        folder: FolderTarget,
    ) -> Option<DropMode> {
        if self == Self::NONE {
            return None;
        }
        let band = match folder {
            FolderTarget::Yes => FOLDER_BAND,
            FolderTarget::No => OTHER_BAND,
        };
        let mode = self.pick(ratio, band);
        self.contains(mode).then_some(mode)
    }

    fn pick(
        self,
        ratio: f32,
        band: f32,
    ) -> DropMode {
        if !self.contains(DropMode::Into) {
            return Self::half(ratio);
        }
        if self == Self::INTO {
            return DropMode::Into;
        }
        if ratio < band {
            return DropMode::Before;
        }
        if ratio > 1.0 - band {
            return DropMode::After;
        }
        DropMode::Into
    }

    fn half(ratio: f32) -> DropMode {
        if ratio < 0.5 {
            return DropMode::Before;
        }
        DropMode::After
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn without_nesting_the_halves_decide() {
        assert_eq!(
            DropModes::REORDER.at(0.49, FolderTarget::No),
            Some(DropMode::Before),
            "upper half"
        );
        assert_eq!(
            DropModes::REORDER.at(0.5, FolderTarget::No),
            Some(DropMode::After),
            "lower half"
        );
    }

    #[test]
    fn only_nesting_always_nests() {
        for ratio in [0.0, 0.5, 1.0] {
            assert_eq!(
                DropModes::INTO.at(ratio, FolderTarget::Yes),
                Some(DropMode::Into),
                "ratio {ratio}"
            );
        }
    }

    #[test]
    fn bands_are_twenty_percent_for_folders_and_thirty_for_the_rest() {
        let cases = [
            (0.19, FolderTarget::Yes, DropMode::Before),
            (0.21, FolderTarget::Yes, DropMode::Into),
            (0.81, FolderTarget::Yes, DropMode::After),
            (0.29, FolderTarget::No, DropMode::Before),
            (0.31, FolderTarget::No, DropMode::Into),
            (0.71, FolderTarget::No, DropMode::After),
        ];
        for (ratio, folder, expected) in cases {
            assert_eq!(
                DropModes::ALL.at(ratio, folder),
                Some(expected),
                "ratio {ratio} {folder:?}"
            );
        }
    }

    #[test]
    fn nothing_allowed_means_no_drop() {
        assert_eq!(DropModes::NONE.at(0.5, FolderTarget::No), None, "refused");
    }
}
