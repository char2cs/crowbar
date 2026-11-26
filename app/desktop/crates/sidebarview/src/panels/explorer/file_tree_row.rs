//! A 26px row of the Files and Changes panels.

pub(crate) mod badges;
pub(crate) mod file_glyph;
mod glyph;
mod row_name;
mod tree_guides;
pub(crate) mod tree_look;

pub(crate) use tree_look::TreeLook;

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ClickEvent, Div, ElementId, FontWeight, InteractiveElement, IntoElement, ParentElement,
    RenderOnce, SharedString, Stateful, StatefulInteractiveElement, Styled, Window, div, px,
};
use sidebar::{Command, CopyNote, Expansion, FileDrop, FileKind, GitStatus, Intent, MenuScope};
use ui::{ContextMenu, Icon, IconName, IconWeight, Palette, Surface, Theme, radius};

use crate::panels::chats::RowInput;
use crate::panels::chats::{ringed, row_focus};
use crate::panels::explorer::file_drag::file_draggable;
use crate::panels::explorer::file_drop;
use crate::panels::explorer::file_menu::file_menu;
use crate::panels::explorer::file_tree_entry::FileTreeEntry;
use crate::panels::explorer::file_tree_row::badges::{
    count_gap, counts_view, labelled, letter_badge, status_color,
};
pub use crate::panels::explorer::file_tree_row::file_glyph::FileGlyph;
pub(crate) use crate::panels::explorer::file_tree_row::glyph::{glyph, pictured};
use crate::panels::explorer::file_tree_row::row_name::shown_name;
pub(crate) use crate::panels::explorer::file_tree_row::tree_guides::tree_guides;
use crate::panels::explorer::search::highlight;
use crate::sidebar::{ICON_MEDIUM, ICON_TINY, SPACE_SM, TEXT_BODY, TEXT_SMALL};
use crate::sidebar::{RowAction, RowHandler};

const BASE_PADDING: f32 = 6.0;
const IGNORED_OPACITY: f32 = 0.5;
const MATCH_TINT: f32 = 0.07;
const CUT_OPACITY: f32 = 0.4;
// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/file-explorer/file-explorer/components/file-explorer-tree-item.tsx
const COPIED_INK: u32 = 0x0022_c55e;

#[derive(IntoElement)]
pub(crate) struct FileTreeRow {
    pub(crate) entry: FileTreeEntry,
    pub(crate) neighbours: (u8, u8),
    pub(crate) input: Option<RowInput>,
    pub(crate) selected: bool,
    pub(crate) cut: bool,
    pub(crate) note: Option<CopyNote>,
    pub(crate) target: Option<FileDrop>,
    pub(crate) look: TreeLook,
    pub(crate) scope: std::rc::Rc<MenuScope>,
    pub(crate) handler: RowHandler,
}

impl FileTreeRow {
    fn filled(
        &self,
        row: Stateful<Div>,
        matched: Option<&(String, std::ops::Range<usize>)>,
        palette: &Palette,
    ) -> Stateful<Div> {
        let row = row.when(self.selected, |row| {
            row.bg(palette.sidebar_element_idle())
                .debug_selector(|| "file-selected".to_owned())
        });
        let row = row.when(matched.is_some() && !self.selected, |row| {
            row.bg(palette.accent().opacity(MATCH_TINT))
        });
        row.when(self.target.is_some(), |row| {
            row.bg(palette.sidebar_drop_nest())
                .border_color(palette.sidebar_drop_nest_edge())
                .text_color(palette.foreground())
                .debug_selector(|| "file-drop-target".to_owned())
        })
    }
}

impl RenderOnce for FileTreeRow {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let mono = Theme::of(cx).mono_family();
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let entry = self.entry.clone();
        let id = ElementId::from(SharedString::from(format!("file-{}", entry.path)));
        let matched = matched_range(&entry.name, &self.look);
        let name = shown_name(
            (&entry, self.input.as_ref()),
            matched.as_ref(),
            (mono, palette),
        );
        let pictured = pictured(&entry, self.input.as_ref());
        let focus = row_focus(&id, window, cx);
        let shaped = frame(id, &entry, (&self.look, palette));
        let tinted = self.filled(shaped, matched.as_ref(), palette);
        let letter = entry.status.letter().map(|l| letter_badge(l, mono));
        let guides = tree_guides((entry.depth, self.neighbours), (&self.look, palette));
        let rung = ui::Pose::from(ui::Modality::rings(&focus, window, cx));
        let row = ringed(tinted, &focus, palette, rung)
            .when(self.cut, |row| row.italic().opacity(CUT_OPACITY))
            .children(guides)
            .child(chevron(&pictured, palette))
            .child(glyph(&pictured, (&self.look, palette), cx))
            .child(labelled(name, letter))
            .children(trailing_counts(&entry, (mono, &self.look), palette))
            .children(self.note.map(copy_note))
            .on_drop(file_drop::on_drop(
                &self.handler,
                file_drop::drop_directory(&entry),
            ))
            .on_click(select_then(&self.handler, entry.clone()));
        if self.input.is_some() {
            return row.into_any_element();
        }
        let row = file_draggable(row, &entry, &self.handler);
        let menu = file_menu(&entry, &self.scope, &self.handler);
        ContextMenu::new(menu_id(&entry), row, menu).into_any_element()
    }
}

fn trailing_counts(
    entry: &FileTreeEntry,
    (mono, look): (&'static str, &TreeLook),
    palette: &Palette,
) -> Option<Div> {
    let counts = entry.counts?;
    Some(counts_view(counts, (mono, count_gap(look)), palette))
}

fn copy_note(note: CopyNote) -> Div {
    div()
        .flex_none()
        .text_size(TEXT_SMALL)
        .text_color(gpui::rgb(COPIED_INK))
        .child(note.text())
}

fn menu_id(entry: &FileTreeEntry) -> SharedString {
    SharedString::from(format!("file-menu-{}", entry.path))
}

pub(crate) fn frame(
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

fn select_then(
    handler: &RowHandler,
    entry: FileTreeEntry,
) -> impl Fn(&ClickEvent, &mut Window, &mut App) + 'static {
    let handler = std::rc::Rc::clone(handler);
    move |event, window, cx| {
        cx.stop_propagation();
        let selected = RowAction::Intent(Intent::SelectFile(entry.path.clone()));
        handler(selected, window, cx);
        let sent = command(&entry, event.click_count());
        handler(RowAction::Intent(Intent::Send(sent)), window, cx);
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

fn faded(status: GitStatus) -> f32 {
    match status {
        GitStatus::Ignored => IGNORED_OPACITY,
        _ => 1.0,
    }
}

fn command(
    entry: &FileTreeEntry,
    clicks: usize,
) -> Command {
    match entry.kind {
        FileKind::Directory(_) => Command::ToggleDirectory(entry.path.clone()),
        FileKind::File if clicks > 1 => Command::OpenFile(entry.path.clone()),
        FileKind::File => Command::PreviewFile(entry.path.clone()),
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;
    use sidebar::{Expansion, FileKind, GitStatus};

    use crate::test_support::{measure, mount};

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

    fn row(entry: FileTreeEntry) -> FileTreeRow {
        FileTreeRow {
            entry,
            neighbours: (0, 0),
            input: None,
            selected: false,
            cut: false,
            note: None,
            target: None,
            look: TreeLook::default(),
            scope: std::rc::Rc::default(),
            handler: std::rc::Rc::new(|_, _, _| {}),
        }
    }

    #[gpui::test]
    fn a_row_is_twenty_six_pixels_tall(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| row(entry(FileKind::File, "main.rs")));
        assert_eq!(f32::from(bounds.size.height), 26.0);
    }

    #[gpui::test]
    fn the_status_letter_follows_the_name_and_the_counts_close_the_row(cx: &mut TestAppContext) {
        let changed = FileTreeEntry {
            status: GitStatus::Modified,
            counts: Some((3, 1)),
            ..entry(FileKind::File, "a.rs")
        };
        let cx = mount(cx, move |_, _| row(changed.clone()));
        let letter = cx.debug_bounds("status-letter").unwrap_or_default();
        let counts = cx.debug_bounds("row-counts").unwrap_or_default();
        let name = cx.debug_bounds("file-name").unwrap_or_default();
        assert_eq!(letter.left() - name.right(), SPACE_SM);
        assert!(
            f32::from(counts.left() - letter.right()) > 100.0,
            "the counts sit at the row end: {counts:?}"
        );
    }

    #[test]
    fn a_directory_toggles_and_a_file_previews_on_a_click_and_opens_on_a_double_click() {
        assert_eq!(
            command(&entry(FileKind::Directory(Expansion::Collapsed), "src"), 1),
            Command::ToggleDirectory("src".into())
        );
        assert_eq!(
            command(&entry(FileKind::File, "a.rs"), 1),
            Command::PreviewFile("a.rs".into())
        );
        assert_eq!(
            command(&entry(FileKind::File, "a.rs"), 2),
            Command::OpenFile("a.rs".into())
        );
    }
}
