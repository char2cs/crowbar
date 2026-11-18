//! A 26px row of the Files and Changes panels.

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ClickEvent, Div, ElementId, FontWeight, HighlightStyle, InteractiveElement, IntoElement,
    ParentElement, RenderOnce, SharedString, Stateful, StatefulInteractiveElement, Styled,
    StyledText, Window, div, px,
};
use sidebar::{Command, Expansion, FileKind, GitStatus, Intent, MenuScope};
use ui::{ContextMenu, Icon, IconName, IconWeight, Palette, Surface, Theme, radius};

use crate::file_badges::{count_gap, counts_view, letter_badge, status_color};
use crate::file_drop;
use crate::file_glyph::{folder_tint, theme_glyph};
use crate::file_menu::file_menu;
use crate::file_search::highlight;
use crate::file_tree_entry::FileTreeEntry;
use crate::row_action::{RowAction, RowHandler};
use crate::row_focus::{ringed, row_focus};
use crate::row_input::RowInput;
use crate::row_label::input_view;
use crate::scale::{ICON_LARGE, ICON_MEDIUM, ICON_TINY, SPACE_SM, TEXT_BODY};
use crate::tree_look::TreeLook;

const BASE_PADDING: f32 = 6.0;
const IGNORED_OPACITY: f32 = 0.5;
const MATCH_TINT: f32 = 0.07;
const HIGHLIGHT_TINT: f32 = 0.3;

#[derive(IntoElement)]
pub(crate) struct FileTreeRow {
    pub(crate) entry: FileTreeEntry,
    pub(crate) input: Option<RowInput>,
    pub(crate) selected: bool,
    pub(crate) look: TreeLook,
    pub(crate) scope: std::rc::Rc<MenuScope>,
    pub(crate) handler: RowHandler,
}

impl RenderOnce for FileTreeRow {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let mono = Theme::of(cx).mono_family();
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let entry = self.entry;
        let id = ElementId::from(SharedString::from(format!("file-{}", entry.path)));
        let action = RowAction::Intent(Intent::Send(command(&entry)));
        let letter = entry.status.letter().map(letter_badge);
        let menu = file_menu(&entry, &self.scope, &self.handler);
        let matched = matched_range(&entry.name, &self.look);
        let name = name_cell(&entry, self.input.as_ref(), (mono, palette));
        let name = match (&matched, self.input.is_some()) {
            (Some((_, range)), false) => highlighted(&entry.name, range.clone(), palette),
            _ => name,
        };
        let focus = row_focus(&id, window, cx);
        let shaped = frame(id, &entry, (&self.look, palette))
            .when(self.selected, |row| row.bg(palette.sidebar_element_idle()));
        let tinted = shaped.when(matched.is_some() && !self.selected, |row| {
            row.bg(palette.accent().opacity(MATCH_TINT))
        });
        let row = ringed(tinted, &focus, palette)
            .child(chevron(&entry, palette))
            .child(glyph(&entry, &self.look))
            .child(name)
            .children(letter)
            .children(
                entry
                    .counts
                    .map(|counts| counts_view(counts, (mono, count_gap(&self.look)), palette)),
            )
            .on_drop(file_drop::on_drop(
                &self.handler,
                file_drop::drop_directory(&entry),
            ))
            .on_click(select_then(&self.handler, (entry.path.clone(), action)));
        if self.input.is_some() {
            return row.into_any_element();
        }
        ContextMenu::new(menu_id(&entry), row, menu).into_any_element()
    }
}

fn menu_id(entry: &FileTreeEntry) -> SharedString {
    SharedString::from(format!("file-menu-{}", entry.path))
}

fn frame(
    id: ElementId,
    entry: &FileTreeEntry,
    (look, palette): (&TreeLook, &Palette),
) -> Stateful<Div> {
    div()
        .id(id)
        .flex()
        .items_center()
        .gap(SPACE_SM)
        .min_h(px(look.height))
        .mx(SPACE_SM)
        .pl(px(BASE_PADDING + f32::from(entry.depth) * look.indent))
        .pr(SPACE_SM)
        .rounded(radius::LG)
        .border_1()
        .border_color(gpui::transparent_black())
        .text_size(TEXT_BODY)
        .font_weight(FontWeight::MEDIUM)
        .text_color(status_color(entry.status, palette))
        .opacity(faded(entry.status))
        .cursor_pointer()
        .hover(move |style| style.bg(palette.sidebar_element_idle()))
}

fn name_cell(
    entry: &FileTreeEntry,
    input: Option<&RowInput>,
    look: (&'static str, &Palette),
) -> Div {
    let cell = div().flex_1().min_w_0();
    match input {
        Some(input) => cell.child(input_view(input, look)),
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

fn matched_range(
    name: &str,
    look: &TreeLook,
) -> Option<(String, std::ops::Range<usize>)> {
    let query = look.query.as_deref()?;
    let range = highlight(name, query)?;
    Some((query.to_owned(), range))
}

fn highlighted(
    name: &str,
    range: std::ops::Range<usize>,
    palette: &Palette,
) -> Div {
    let style = HighlightStyle {
        background_color: Some(palette.accent().opacity(HIGHLIGHT_TINT)),
        ..HighlightStyle::default()
    };
    let text = StyledText::new(name.to_owned()).with_highlights([(range, style)]);
    div().flex_1().min_w_0().truncate().child(text)
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

fn glyph(
    entry: &FileTreeEntry,
    look: &TreeLook,
) -> Div {
    let name = theme_glyph(look.icons, entry.kind, &entry.name);
    let icon = Icon::new(name).size(ICON_MEDIUM);
    let tint = folder_tint(look.icons).filter(|_| matches!(entry.kind, FileKind::Directory(_)));
    let icon = match tint {
        Some(rgb) => icon.color(gpui::rgb(rgb).into()),
        None => icon,
    };
    div()
        .flex()
        .flex_none()
        .size(ICON_LARGE)
        .items_center()
        .justify_center()
        .child(icon)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;
    use sidebar::{Expansion, FileKind, GitStatus};

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
            look: TreeLook::default(),
            scope: std::rc::Rc::default(),
            handler: std::rc::Rc::new(|_, _, _| {}),
        });
        assert_eq!(f32::from(bounds.size.height), 26.0);
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
