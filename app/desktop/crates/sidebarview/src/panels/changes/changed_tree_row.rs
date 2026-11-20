//! A row of the Changes tree: a folder that folds, or a changed file that opens the review.

use gpui::{
    App, ClickEvent, ElementId, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    SharedString, StatefulInteractiveElement, Styled, Window, div,
};
use sidebar::{ChangedRow, Command, FileKind, GitStatus, Intent};
use ui::{Badge, BadgeSize, BadgeVariant, Surface, Theme, Tooltip};

use crate::panels::explorer::badges::{count_gap, counts_view, letter_badge};
use crate::panels::explorer::{FileTreeEntry, TreeLook, frame, glyph};
use crate::sidebar::{RowAction, RowHandler, SPACE_SM};

#[derive(IntoElement)]
pub(crate) struct ChangedTreeRow {
    pub(crate) row: ChangedRow,
    pub(crate) look: TreeLook,
    pub(crate) handler: RowHandler,
}

fn entry(row: &ChangedRow) -> FileTreeEntry {
    match row {
        ChangedRow::Folder {
            path,
            label,
            depth,
            expansion,
        } => FileTreeEntry {
            path: path.clone(),
            name: label.clone(),
            depth: *depth,
            kind: FileKind::Directory(*expansion),
            status: GitStatus::Clean,
            counts: None,
        },
        ChangedRow::File {
            file, name, depth, ..
        } => FileTreeEntry {
            path: file.path.clone(),
            name: name.clone(),
            depth: *depth,
            kind: FileKind::File,
            status: file.status,
            counts: (file.added + file.deleted > 0).then_some((file.added, file.deleted)),
        },
    }
}

fn intent(row: &ChangedRow) -> Intent {
    match row {
        ChangedRow::Folder { path, .. } => Intent::ToggleChangedFold(path.clone()),
        ChangedRow::File { .. } => Intent::Send(Command::OpenReview),
    }
}

fn threads(row: &ChangedRow) -> u32 {
    match row {
        ChangedRow::File { threads, .. } => *threads,
        ChangedRow::Folder { .. } => 0,
    }
}

fn on_click(
    handler: &RowHandler,
    intent: Intent,
) -> impl Fn(&ClickEvent, &mut Window, &mut App) + 'static {
    let handler = std::rc::Rc::clone(handler);
    move |_, window, cx| {
        cx.stop_propagation();
        handler(RowAction::Intent(intent.clone()), window, cx);
    }
}

fn thread_badge(open: u32) -> Option<Badge> {
    (open > 0).then(|| {
        Badge::new(open.to_string())
            .variant(BadgeVariant::Info)
            .size(BadgeSize::Sm)
            .on(Surface::Chrome)
    })
}

impl RenderOnce for ChangedTreeRow {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let mono = Theme::of(cx).mono_family();
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let entry = entry(&self.row);
        let id = ElementId::from(SharedString::from(format!("changed-{}", entry.path)));
        let letter = entry
            .status
            .letter()
            .map(|letter| letter_badge(letter, mono));
        let counts = entry
            .counts
            .map(|counts| counts_view(counts, (mono, count_gap(&self.look)), palette));
        let badge = thread_badge(threads(&self.row));
        let trailing = div()
            .ml_auto()
            .flex()
            .flex_none()
            .items_center()
            .gap(SPACE_SM)
            .children(counts)
            .children(badge);
        let selector = format!("changed-row-{}", entry.path);
        let row = frame(id, &entry, (&self.look, palette))
            .debug_selector(|| selector)
            .child(glyph(&entry, &self.look))
            .child(div().min_w_0().truncate().child(entry.name.clone()))
            .children(letter)
            .child(trailing)
            .on_click(on_click(&self.handler, intent(&self.row)));
        match self.row {
            ChangedRow::File { file, .. } => Tooltip::new(
                SharedString::from(format!("changed-tip-{}", file.path)),
                file.path,
                row,
            )
            .into_any_element(),
            ChangedRow::Folder { .. } => row.into_any_element(),
        }
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{ChangedFile, Expansion};

    use super::*;

    fn file(
        added: u32,
        deleted: u32,
    ) -> ChangedRow {
        let file = ChangedFile {
            path: "src/a.rs".into(),
            status: GitStatus::Added,
            added,
            deleted,
        };
        ChangedRow::File {
            file,
            name: "a.rs".into(),
            depth: 1,
            threads: 2,
        }
    }

    #[test]
    fn a_file_carries_its_line_counts_and_opens_the_review_and_a_folder_folds() {
        assert_eq!(entry(&file(3, 1)).counts, Some((3, 1)));
        assert_eq!(entry(&file(0, 0)).counts, None);
        assert_eq!(intent(&file(3, 1)), Intent::Send(Command::OpenReview));
        let folder = ChangedRow::Folder {
            path: "src".into(),
            label: "src".into(),
            depth: 0,
            expansion: Expansion::Expanded,
        };
        assert_eq!(intent(&folder), Intent::ToggleChangedFold("src".into()));
        assert_eq!((threads(&file(1, 0)), threads(&folder)), (2, 0));
    }
}
