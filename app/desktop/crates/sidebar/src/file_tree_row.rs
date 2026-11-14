//! A 26px row of the Files and Changes panels.

use gpui::{
    App, Div, ElementId, FontWeight, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::font::MONO_FAMILY;
use ui::{Icon, IconName, IconWeight, Palette, Surface, Theme, radius};

use crate::command::Command;
use crate::expansion::Expansion;
use crate::file_kind::FileKind;
use crate::file_tree_entry::FileTreeEntry;
use crate::git_status::GitStatus;
use crate::intent::Intent;
use crate::row_action::{RowAction, RowHandler};
use crate::row_report::report;

const ROW_HEIGHT: f32 = 26.0;
const INDENT: f32 = 14.0;
const BASE_PADDING: f32 = 6.0;
const IGNORED_OPACITY: f32 = 0.5;

#[derive(IntoElement)]
pub(crate) struct FileTreeRow {
    pub(crate) entry: FileTreeEntry,
    pub(crate) handler: RowHandler,
}

impl RenderOnce for FileTreeRow {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let entry = self.entry;
        let id = ElementId::from(SharedString::from(format!("file-{}", entry.path)));
        let action = RowAction::Intent(Intent::Send(command(&entry)));
        let faded = match entry.status {
            GitStatus::Ignored => IGNORED_OPACITY,
            _ => 1.0,
        };
        let letter = entry.status.letter().map(letter_badge);
        div()
            .id(id)
            .flex()
            .items_center()
            .gap(px(6.0))
            .h(px(ROW_HEIGHT))
            .mx(px(6.0))
            .pl(px(BASE_PADDING + f32::from(entry.depth) * INDENT))
            .pr(px(6.0))
            .rounded(radius::LG)
            .border_1()
            .border_color(gpui::transparent_black())
            .text_size(px(13.0))
            .font_weight(FontWeight::MEDIUM)
            .text_color(status_color(entry.status, palette))
            .opacity(faded)
            .cursor_pointer()
            .hover(move |style| style.bg(palette.sidebar_element_idle()))
            .child(chevron(&entry, palette))
            .child(glyph(&entry))
            .child(
                div()
                    .flex_1()
                    .min_w_0()
                    .truncate()
                    .child(entry.name.clone()),
            )
            .children(letter)
            .children(entry.counts.map(|counts| counts_view(counts, palette)))
            .on_click(report(&self.handler, action))
    }
}

fn command(entry: &FileTreeEntry) -> Command {
    match entry.kind {
        FileKind::Directory(_) => Command::ToggleDirectory(entry.path.clone()),
        FileKind::File => Command::OpenFile(entry.path.clone()),
    }
}

fn status_color(status: GitStatus, palette: &Palette) -> gpui::Hsla {
    match status {
        GitStatus::Added => palette.git_added(),
        GitStatus::Untracked => palette.git_untracked(),
        GitStatus::Modified => palette.git_modified(),
        GitStatus::Deleted => palette.git_deleted(),
        GitStatus::Clean | GitStatus::Ignored => palette.foreground(),
    }
}

fn chevron(entry: &FileTreeEntry, palette: &Palette) -> Div {
    let turn = match entry.kind {
        FileKind::Directory(Expansion::Expanded) => std::f32::consts::FRAC_PI_2,
        FileKind::Directory(Expansion::Collapsed) | FileKind::File => 0.0,
    };
    let caret = Icon::new(IconName::CaretRight)
        .weight(IconWeight::Bold)
        .size(px(10.0))
        .rotate(turn);
    let frame = div()
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .size(px(14.0))
        .text_color(palette.muted_foreground());
    match entry.kind {
        FileKind::Directory(_) => frame.child(caret),
        FileKind::File => frame,
    }
}

fn glyph(entry: &FileTreeEntry) -> Div {
    let name = match entry.kind {
        FileKind::Directory(Expansion::Expanded) => IconName::FolderOpen,
        FileKind::Directory(Expansion::Collapsed) => IconName::Folder,
        FileKind::File => IconName::Code,
    };
    div()
        .flex()
        .flex_none()
        .size(px(16.0))
        .items_center()
        .justify_center()
        .child(Icon::new(name).size(px(14.0)))
}

fn letter_badge(letter: char) -> Div {
    div()
        .flex_none()
        .text_size(px(11.0))
        .child(letter.to_string())
}

fn counts_view((added, deleted): (u32, u32), palette: &Palette) -> Div {
    div()
        .flex()
        .gap(px(4.0))
        .font_family(MONO_FAMILY)
        .text_size(px(11.0))
        .child(
            div()
                .text_color(palette.sidebar_count_added())
                .child(format!("+{added}")),
        )
        .child(
            div()
                .text_color(palette.sidebar_count_deleted())
                .child(format!("-{deleted}")),
        )
}

#[cfg(test)]
#[path = "file_tree_row_tests.rs"]
mod tests;
