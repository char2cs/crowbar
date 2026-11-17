//! One row of the tree: an indent, a caret and icon for a folder, and the name or note.

use gpui::{
    App, Div, ElementId, Hsla, InteractiveElement, IntoElement, ParentElement, SharedString,
    StatefulInteractiveElement, Styled, Window, div,
};
use shell_data::EntryKind;
use ui::{Icon, IconName, Palette, Pose};

use crate::folder_note::FolderNote;
use crate::folder_row::FolderRow;
use crate::row_look::{GAP, ICON, INDENT, INERT_OPACITY, ROW_HEIGHT, TEXT};

pub(crate) type Toggle = Box<dyn Fn(&mut Window, &mut App)>;

fn base(level: usize) -> Div {
    let depth = u8::try_from(level.saturating_sub(1)).unwrap_or(u8::MAX);
    div()
        .flex()
        .items_center()
        .gap(GAP)
        .h(ROW_HEIGHT)
        .pl(INDENT * f32::from(depth))
        .text_size(TEXT)
}

fn caret(
    index: usize,
    open: Pose,
    toggle: Toggle,
) -> impl IntoElement {
    let name = match open {
        Pose::On => IconName::CaretDown,
        Pose::Off => IconName::CaretRight,
    };
    div()
        .id(ElementId::from(("folder-caret", index)))
        .on_click(move |_, window, cx| toggle(window, cx))
        .child(Icon::new(name).size(ICON))
}

fn folder_icon(open: Pose) -> Icon {
    let name = match open {
        Pose::On => IconName::FolderOpen,
        Pose::Off => IconName::Folder,
    };
    Icon::new(name).size(ICON)
}

fn note_color(
    note: &FolderNote,
    palette: &Palette,
) -> Hsla {
    if matches!(note, FolderNote::Failed(_)) {
        return palette.destructive();
    }
    palette.muted_foreground()
}

pub(crate) fn tree_row(
    index: usize,
    row: &FolderRow,
    look: (&str, &Palette),
    toggle: Toggle,
) -> Div {
    let (host, palette) = look;
    match row {
        FolderRow::Note { level, note } => base(*level + 1)
            .text_color(note_color(note, palette))
            .child(SharedString::from(note.text(host))),
        FolderRow::Entry { entry, level, open } if entry.kind == EntryKind::Folder => base(*level)
            .child(caret(index, *open, toggle))
            .child(folder_icon(*open))
            .child(SharedString::from(entry.name.clone())),
        FolderRow::Entry { entry, level, .. } => base(*level)
            .opacity(INERT_OPACITY)
            .pl(INDENT * 2.0)
            .child(SharedString::from(entry.name.clone())),
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use ui::{Surface, Theme};

    use super::*;

    #[gpui::test]
    fn only_a_failure_is_drawn_in_the_alarm_colour(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let palette = cx.update(|cx| *Theme::of(cx).palette(Surface::Window));
        let failed = note_color(&FolderNote::Failed("no".to_owned()), &palette);
        let empty = note_color(&FolderNote::Empty, &palette);
        assert_eq!(failed, palette.destructive());
        assert_eq!(empty, palette.muted_foreground());
    }
}
