//! The state of a branch's workspace as the sidebar glyph shows it.

use ui::{IconName, IconWeight};

/// Lifecycle and pull-request state of a landed workspace.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum WorkspaceStatus {
    /// Needs provisioning before it can be used.
    Placeholder,
    /// Locked against changes.
    Locked,
    /// A fresh branch.
    New,
    /// An open pull request with conflicts.
    PrConflicts,
    /// The branch was deleted.
    Deleted,
    /// The pull request was closed.
    PrClosed,
    /// The pull request is open.
    PrOpen,
    /// The pull request was merged.
    PrMerged,
}

impl WorkspaceStatus {
    pub(crate) fn glyph(self) -> (IconName, IconWeight) {
        match self {
            Self::Placeholder | Self::PrConflicts => (IconName::Warning, IconWeight::Fill),
            Self::Locked => (IconName::Lock, IconWeight::Fill),
            Self::New => (IconName::GitBranch, IconWeight::Fill),
            Self::Deleted | Self::PrClosed => (IconName::GitFork, IconWeight::Fill),
            Self::PrOpen => (IconName::GitPullRequest, IconWeight::Fill),
            Self::PrMerged => (IconName::GitMerge, IconWeight::Fill),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_status_has_the_spec_glyph() {
        let cases = [
            (WorkspaceStatus::Placeholder, IconName::Warning),
            (WorkspaceStatus::Locked, IconName::Lock),
            (WorkspaceStatus::New, IconName::GitBranch),
            (WorkspaceStatus::PrConflicts, IconName::Warning),
            (WorkspaceStatus::Deleted, IconName::GitFork),
            (WorkspaceStatus::PrClosed, IconName::GitFork),
            (WorkspaceStatus::PrOpen, IconName::GitPullRequest),
            (WorkspaceStatus::PrMerged, IconName::GitMerge),
        ];
        for (status, icon) in cases {
            assert_eq!(status.glyph(), (icon, IconWeight::Fill), "{status:?}");
            assert!(
                ui::Icon::path(icon, IconWeight::Fill).is_some(),
                "{icon:?} fill is embedded"
            );
        }
    }
}
