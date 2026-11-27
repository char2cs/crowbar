mod glyph_tone;

use sidebar::{Lock, Row, WorkspaceStatus, Worktree};
use ui::{IconName, IconWeight};

pub use glyph_tone::GlyphTone;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct ChatGlyph {
    pub icon: IconName,
    pub weight: IconWeight,
    pub tone: GlyphTone,
}

impl ChatGlyph {
    pub fn of(row: &Row) -> Self {
        if let Some(workspace) = &row.workspace {
            return Self::toned(status_icon(workspace.status));
        }
        if row.lock == Lock::Locked {
            return Self::fill(IconName::Lock);
        }
        if row.worktree == Worktree::Owned {
            return Self::fill(IconName::GitBranch);
        }
        Self {
            icon: IconName::ChatsCircle,
            weight: IconWeight::Regular,
            tone: GlyphTone::Plain,
        }
    }

    fn toned((icon, tone): (IconName, GlyphTone)) -> Self {
        Self {
            icon,
            weight: IconWeight::Fill,
            tone,
        }
    }

    fn fill(icon: IconName) -> Self {
        Self::toned((icon, GlyphTone::Plain))
    }
}

fn status_icon(status: WorkspaceStatus) -> (IconName, GlyphTone) {
    match status {
        WorkspaceStatus::Placeholder | WorkspaceStatus::PrConflicts => {
            (IconName::Warning, GlyphTone::Attention)
        },
        WorkspaceStatus::Locked => (IconName::Lock, GlyphTone::Plain),
        WorkspaceStatus::New => (IconName::GitBranch, GlyphTone::Plain),
        WorkspaceStatus::Deleted | WorkspaceStatus::PrClosed => {
            (IconName::GitFork, GlyphTone::Closed)
        },
        WorkspaceStatus::PrOpen => (IconName::GitPullRequest, GlyphTone::Open),
        WorkspaceStatus::PrMerged => (IconName::GitMerge, GlyphTone::Merged),
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{Activity, RowKind, Workspace};

    use super::*;

    fn branch_in(status: WorkspaceStatus) -> Row {
        let mut row = Row::new("b", RowKind::Branch, 1, "main");
        row.workspace = Some(Workspace {
            status,
            branch: "x".into(),
            added: 0,
            deleted: 0,
            held_by_path: None,
        });
        row
    }

    #[test]
    fn a_working_chat_still_names_its_idle_glyph() {
        let mut row = branch_in(WorkspaceStatus::PrMerged);
        row.activity = Activity::Working;
        let merged = ChatGlyph {
            icon: IconName::GitMerge,
            weight: IconWeight::Fill,
            tone: GlyphTone::Merged,
        };
        assert_eq!(ChatGlyph::of(&row), merged);
    }

    #[test]
    fn a_plain_chat_shows_the_bubble() {
        let row = Row::new("c", RowKind::Chat, 1, "hi");
        assert_eq!(ChatGlyph::of(&row).icon, IconName::ChatsCircle);
    }

    #[test]
    fn every_status_has_the_spec_glyph() {
        use GlyphTone::{Attention, Closed, Merged, Open, Plain};
        use WorkspaceStatus as Status;
        let cases = [
            (Status::Placeholder, IconName::Warning, Attention),
            (Status::Locked, IconName::Lock, Plain),
            (Status::New, IconName::GitBranch, Plain),
            (Status::PrConflicts, IconName::Warning, Attention),
            (Status::Deleted, IconName::GitFork, Closed),
            (Status::PrClosed, IconName::GitFork, Closed),
            (Status::PrOpen, IconName::GitPullRequest, Open),
            (Status::PrMerged, IconName::GitMerge, Merged),
        ];
        for (status, icon, tone) in cases {
            let row = branch_in(status);
            let want = ChatGlyph::toned((icon, tone));
            assert_eq!(ChatGlyph::of(&row), want, "{status:?}");
            assert!(
                ui::Icon::path(icon, IconWeight::Fill).is_some(),
                "{icon:?} fill is embedded"
            );
        }
    }
}
