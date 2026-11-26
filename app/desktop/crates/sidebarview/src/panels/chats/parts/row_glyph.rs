//! Which glyph leads a tree row.

pub(super) mod chat_glyph;

use ui::{IconName, IconWeight};

use sidebar::{Activity, Expansion, Row, RowKind};

use crate::panels::chats::parts::row_glyph::chat_glyph::{ChatGlyph, GlyphTone};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RowGlyph {
    Spinner,
    Mark,
    Icon(IconName, IconWeight, GlyphTone),
}

impl RowGlyph {
    pub(crate) fn of(
        row: &Row,
        expansion: Expansion,
    ) -> Self {
        if row.activity == Activity::Working {
            return Self::Spinner;
        }
        if row.kind == RowKind::RepoHeader {
            return Self::Mark;
        }
        if row.kind == RowKind::Folder {
            return Self::folder(expansion);
        }
        let glyph = ChatGlyph::of(row);
        Self::Icon(glyph.icon, glyph.weight, glyph.tone)
    }

    fn folder(expansion: Expansion) -> Self {
        let name = match expansion {
            Expansion::Expanded => IconName::FolderOpen,
            Expansion::Collapsed => IconName::Folder,
        };
        Self::Icon(name, IconWeight::Duotone, GlyphTone::Plain)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::{Lock, Workspace, WorkspaceStatus, Worktree};

    fn working_repo() -> Row {
        let mut row = Row::new("r", RowKind::RepoHeader, 0, "demo");
        row.activity = Activity::Working;
        row
    }

    fn folder() -> Row {
        Row::new("f", RowKind::Folder, 1, "src")
    }

    fn locked_branch() -> Row {
        let mut row = Row::new("b", RowKind::Branch, 1, "main");
        row.lock = Lock::Locked;
        row
    }

    fn worktree_branch() -> Row {
        let mut row = Row::new("b", RowKind::Branch, 1, "main");
        row.worktree = Worktree::Owned;
        row
    }

    fn merged_branch() -> Row {
        let mut row = worktree_branch();
        row.workspace = Some(Workspace {
            status: WorkspaceStatus::PrMerged,
            branch: "x".into(),
            added: 0,
            deleted: 0,
            held_by_path: None,
        });
        row
    }

    fn chat() -> Row {
        Row::new("c", RowKind::Chat, 1, "hi")
    }

    type Case = (&'static str, fn() -> Row, Expansion, RowGlyph);

    const CASES: &[Case] = &[
        (
            "working beats everything",
            working_repo,
            Expansion::Expanded,
            RowGlyph::Spinner,
        ),
        (
            "closed folder",
            folder,
            Expansion::Collapsed,
            RowGlyph::Icon(IconName::Folder, IconWeight::Duotone, GlyphTone::Plain),
        ),
        (
            "open folder",
            folder,
            Expansion::Expanded,
            RowGlyph::Icon(IconName::FolderOpen, IconWeight::Duotone, GlyphTone::Plain),
        ),
        (
            "locked without workspace",
            locked_branch,
            Expansion::Expanded,
            RowGlyph::Icon(IconName::Lock, IconWeight::Fill, GlyphTone::Plain),
        ),
        (
            "owns a worktree",
            worktree_branch,
            Expansion::Expanded,
            RowGlyph::Icon(IconName::GitBranch, IconWeight::Fill, GlyphTone::Plain),
        ),
        (
            "the workspace status wins",
            merged_branch,
            Expansion::Expanded,
            RowGlyph::Icon(IconName::GitMerge, IconWeight::Fill, GlyphTone::Merged),
        ),
        (
            "a plain chat shows the bubbles",
            chat,
            Expansion::Expanded,
            RowGlyph::Icon(IconName::ChatsCircle, IconWeight::Regular, GlyphTone::Plain),
        ),
    ];

    #[test]
    fn the_glyph_follows_activity_kind_status_lock_and_worktree() {
        for (name, build, expansion, expected) in CASES {
            assert_eq!(RowGlyph::of(&build(), *expansion), *expected, "{name}");
        }
    }
}
