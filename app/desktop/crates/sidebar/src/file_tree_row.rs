//! A 26px row of the Files and Changes panels.

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ClickEvent, Div, ElementId, FontWeight, InteractiveElement, IntoElement, ParentElement,
    RenderOnce, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::font::MONO_FAMILY;
use ui::{ContextMenu, Icon, IconName, IconWeight, Palette, Surface, Theme, radius};

use crate::command::Command;
use crate::expansion::Expansion;
use crate::file_kind::FileKind;
use crate::file_menu::file_menu;
use crate::file_tree_entry::FileTreeEntry;
use crate::git_status::GitStatus;
use crate::intent::Intent;
use crate::menu_scope::MenuScope;
use crate::row_action::{RowAction, RowHandler};
use crate::row_input::RowInput;
use crate::row_label::input_view;
use crate::scale::{
    ICON_LARGE, ICON_MEDIUM, ICON_TINY, SPACE_SM, SPACE_XS, TEXT_BODY, TEXT_CAPTION,
};

const ROW_HEIGHT: f32 = 26.0;
const INDENT: f32 = 14.0;
const BASE_PADDING: f32 = 6.0;
const IGNORED_OPACITY: f32 = 0.5;

#[derive(IntoElement)]
pub(crate) struct FileTreeRow {
    pub(crate) entry: FileTreeEntry,
    pub(crate) input: Option<RowInput>,
    pub(crate) selected: bool,
    pub(crate) scope: std::rc::Rc<MenuScope>,
    pub(crate) handler: RowHandler,
}

impl RenderOnce for FileTreeRow {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let entry = self.entry;
        let id = ElementId::from(SharedString::from(format!("file-{}", entry.path)));
        let menu_id = SharedString::from(format!("file-menu-{}", entry.path));
        let action = RowAction::Intent(Intent::Send(command(&entry)));
        let letter = entry.status.letter().map(letter_badge);
        let menu = file_menu(&entry, &self.scope, &self.handler);
        let editing = self.input.is_some();
        let name = name_cell(&entry, self.input.as_ref(), palette);
        let row = div()
            .id(id)
            .flex()
            .items_center()
            .gap(SPACE_SM)
            .min_h(px(ROW_HEIGHT))
            .mx(SPACE_SM)
            .pl(px(BASE_PADDING + f32::from(entry.depth) * INDENT))
            .pr(SPACE_SM)
            .rounded(radius::LG)
            .border_1()
            .border_color(gpui::transparent_black())
            .text_size(TEXT_BODY)
            .font_weight(FontWeight::MEDIUM)
            .text_color(status_color(entry.status, palette))
            .opacity(faded(entry.status))
            .cursor_pointer()
            .when(self.selected, |row| row.bg(palette.sidebar_element_idle()))
            .hover(move |style| style.bg(palette.sidebar_element_idle()))
            .child(chevron(&entry, palette))
            .child(glyph(&entry))
            .child(name)
            .children(letter)
            .children(entry.counts.map(|counts| counts_view(counts, palette)))
            .on_click(select_then(&self.handler, (entry.path.clone(), action)));
        if editing {
            return row.into_any_element();
        }
        ContextMenu::new(menu_id, row, menu).into_any_element()
    }
}

fn name_cell(
    entry: &FileTreeEntry,
    input: Option<&RowInput>,
    palette: &Palette,
) -> Div {
    let cell = div().flex_1().min_w_0();
    match input {
        Some(input) => cell.child(input_view(input, palette)),
        None => cell.truncate().child(entry.name.clone()),
    }
}

fn select_then(
    handler: &RowHandler,
    (path, action): (String, RowAction),
) -> impl Fn(&ClickEvent, &mut Window, &mut App) + 'static {
    let handler = std::rc::Rc::clone(handler);
    move |_, window, cx| {
        cx.stop_propagation();
        handler(
            RowAction::Intent(Intent::SelectFile(path.clone())),
            window,
            cx,
        );
        handler(action.clone(), window, cx);
    }
}

fn faded(status: GitStatus) -> f32 {
    match status {
        GitStatus::Ignored => IGNORED_OPACITY,
        _ => 1.0,
    }
}

fn command(entry: &FileTreeEntry) -> Command {
    match entry.kind {
        FileKind::Directory(_) => Command::ToggleDirectory(entry.path.clone()),
        FileKind::File => Command::OpenFile(entry.path.clone()),
    }
}

fn status_color(
    status: GitStatus,
    palette: &Palette,
) -> gpui::Hsla {
    match status {
        GitStatus::Added => palette.git_added(),
        GitStatus::Untracked => palette.git_untracked(),
        GitStatus::Modified => palette.git_modified(),
        GitStatus::Deleted => palette.git_deleted(),
        GitStatus::Clean | GitStatus::Ignored => palette.foreground(),
    }
}

fn chevron(
    entry: &FileTreeEntry,
    palette: &Palette,
) -> Div {
    let turn = match entry.kind {
        FileKind::Directory(Expansion::Expanded) => std::f32::consts::FRAC_PI_2,
        FileKind::Directory(Expansion::Collapsed) | FileKind::File => 0.0,
    };
    let caret = Icon::new(IconName::CaretRight)
        .weight(IconWeight::Bold)
        .size(ICON_TINY)
        .rotate(turn);
    let frame = div()
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .size(ICON_MEDIUM)
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
        .size(ICON_LARGE)
        .items_center()
        .justify_center()
        .child(Icon::new(name).size(ICON_MEDIUM))
}

fn letter_badge(letter: char) -> Div {
    div()
        .flex_none()
        .text_size(TEXT_CAPTION)
        .child(letter.to_string())
}

fn counts_view(
    (added, deleted): (u32, u32),
    palette: &Palette,
) -> Div {
    div()
        .flex()
        .gap(SPACE_XS)
        .font_family(MONO_FAMILY)
        .text_size(TEXT_CAPTION)
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
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    use crate::expansion::Expansion;
    use crate::file_kind::FileKind;
    use crate::git_status::GitStatus;
    use crate::test_support::measure;

    fn entry(
        kind: FileKind,
        path: &str,
    ) -> FileTreeEntry {
        FileTreeEntry {
            path: path.into(),
            name: path.into(),
            depth: 0,
            kind,
            status: GitStatus::Clean,
            counts: None,
        }
    }

    #[gpui::test]
    fn a_row_is_twenty_six_pixels_tall(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| FileTreeRow {
            entry: entry(FileKind::File, "main.rs"),
            input: None,
            selected: false,
            scope: std::rc::Rc::default(),
            handler: std::rc::Rc::new(|_, _, _| {}),
        });
        assert_eq!(f32::from(bounds.size.height), ROW_HEIGHT);
    }

    #[test]
    fn a_directory_toggles_and_a_file_opens() {
        assert_eq!(
            command(&entry(FileKind::Directory(Expansion::Collapsed), "src")),
            Command::ToggleDirectory("src".into())
        );
        assert_eq!(
            command(&entry(FileKind::File, "a.rs")),
            Command::OpenFile("a.rs".into())
        );
    }
}
