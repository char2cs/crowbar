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
