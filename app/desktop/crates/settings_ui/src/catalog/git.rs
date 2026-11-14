//! The Git page.

use super::rows::row;
use crate::control::Control;
use crate::page::Page;
use crate::section::Section;

pub(super) fn page() -> Page {
    Page::new("git", "Git", "git-branch")
        .block(Section::new("Git View").headerless().row(row(
            ("compactGitStatusBadges", "Compact Git Status Badges"),
            "Use a denser layout for diff stats and staged labels in the Git panel",
            Control::switch(),
            false,
        )))
        .block(Section::new("File Tree").row(row(
            ("showGitStatusInFileTree", "Show Git Status In File Tree"),
            "Display Git color decorations in Files",
            Control::switch(),
            true,
        )))
}

#[cfg(test)]
#[path = "git_tests.rs"]
mod tests;
