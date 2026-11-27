use gpui::{
    App, ClickEvent, Div, ElementId, Hsla, InteractiveElement, IntoElement, ParentElement,
    RenderOnce, SharedString, StatefulInteractiveElement, Styled, Window, div,
};
use sidebar::{ChangedRow, Command, FileKind, GitStatus, Intent};
use ui::{Badge, BadgeSize, BadgeVariant, Palette, Surface, Theme, Tooltip};

use crate::panels::changes::viewed_box::viewed_box;
use crate::panels::explorer::badges::{
    count_gap, counts_view, labelled, letter_badge, status_color,
};
use crate::panels::explorer::{FileTreeEntry, TreeLook, frame, glyph, tree_guides};
use crate::sidebar::{RowAction, RowHandler, SPACE_SM};

#[derive(IntoElement)]
pub(crate) struct ChangedTreeRow {
    pub(crate) row: ChangedRow,
    pub(crate) neighbours: (u8, u8),
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
            ..
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
        ChangedRow::File { file, .. } => Intent::Send(Command::OpenReview {
            path: file.path.clone(),
        }),
    }
}

fn name_color(
    row: &ChangedRow,
    palette: &Palette,
) -> Hsla {
    match row {
        ChangedRow::File { viewed: true, .. } => palette.muted_foreground(),
        ChangedRow::File { file, .. } => status_color(file.status, palette),
        ChangedRow::Folder { .. } => palette.foreground(),
    }
}

fn tally(
    row: &ChangedRow,
    palette: &Palette,
) -> Option<Div> {
    let ChangedRow::Folder { files, viewed, .. } = row else {
        return None;
    };
    let text = format!("{viewed}/{}", files.len());
    Some(div().text_color(palette.muted_foreground()).child(text))
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

impl ChangedTreeRow {
    fn trailing(
        &self,
        entry: &FileTreeEntry,
        (mono, palette): (&'static str, &Palette),
    ) -> Div {
        let counts = entry
            .counts
            .map(|counts| counts_view(counts, (mono, count_gap(&self.look)), palette));
        div()
            .flex()
            .flex_none()
            .items_center()
            .gap(SPACE_SM)
            .children(tally(&self.row, palette))
            .children(counts)
            .children(thread_badge(threads(&self.row)))
    }

    fn name(
        &self,
        entry: &FileTreeEntry,
        palette: &Palette,
    ) -> Div {
        let selector = format!("changed-name-{}", entry.path);
        div()
            .debug_selector(|| selector)
            .min_w_0()
            .truncate()
            .text_color(name_color(&self.row, palette))
            .child(entry.name.clone())
    }
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
        let letter = entry.status.letter().map(|l| letter_badge(l, mono));
        let selector = format!("changed-row-{}", entry.path);
        let row = frame(id, &entry, (&self.look, palette))
            .debug_selector(|| selector)
            .children(tree_guides(
                (entry.depth, self.neighbours),
                (&self.look, palette),
            ))
            .child(viewed_box(&self.row, &self.handler))
            .child(glyph(&entry, (&self.look, palette), cx))
            .child(labelled(self.name(&entry, palette), letter))
            .child(self.trailing(&entry, (mono, palette)))
            .on_click(on_click(&self.handler, intent(&self.row)));
        let ChangedRow::File { file, .. } = self.row else {
            return row.into_any_element();
        };
        let tip = SharedString::from(format!("changed-tip-{}", file.path));
        Tooltip::new(tip, file.path, row).into_any_element()
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
            ..ChangedFile::default()
        };
        ChangedRow::File {
            file,
            name: "a.rs".into(),
            depth: 1,
            threads: 2,
            viewed: false,
        }
    }

    #[gpui::test]
    fn the_name_and_its_letter_keep_their_gap_beside_the_trailing_counts(
        cx: &mut gpui::TestAppContext
    ) {
        let cx = crate::test_support::mount(cx, |_, _| ChangedTreeRow {
            row: file(3, 1),
            neighbours: (0, 0),
            look: TreeLook::default(),
            handler: std::rc::Rc::new(|_, _, _| {}),
        });
        let name = cx.debug_bounds("changed-name-src/a.rs").unwrap_or_default();
        let letter = cx.debug_bounds("status-letter").unwrap_or_default();
        assert_eq!(letter.left() - name.right(), SPACE_SM);
    }

    #[test]
    fn a_file_carries_its_line_counts_and_opens_the_review_and_a_folder_folds() {
        assert_eq!(entry(&file(3, 1)).counts, Some((3, 1)));
        assert_eq!(entry(&file(0, 0)).counts, None);
        let open = Command::OpenReview {
            path: "src/a.rs".into(),
        };
        assert_eq!(intent(&file(3, 1)), Intent::Send(open));
        let folder = ChangedRow::Folder {
            path: "src".into(),
            label: "src".into(),
            depth: 0,
            expansion: Expansion::Expanded,
            files: Vec::new(),
            viewed: 0,
        };
        assert_eq!(intent(&folder), Intent::ToggleChangedFold("src".into()));
        assert_eq!((threads(&file(1, 0)), threads(&folder)), (2, 0));
    }
}
