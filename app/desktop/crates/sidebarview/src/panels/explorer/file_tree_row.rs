//! A 26px row of the Files and Changes panels.

pub(crate) mod badges;
pub(crate) mod file_glyph;
mod glyph;
mod tree_guides;
pub(crate) mod tree_look;

pub(crate) use tree_look::TreeLook;

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ClickEvent, Div, ElementId, FontWeight, HighlightStyle, InteractiveElement, IntoElement,
    ParentElement, RenderOnce, SharedString, Stateful, StatefulInteractiveElement, Styled,
    StyledText, Window, div, px,
};
use sidebar::{Command, Expansion, FileKind, GitStatus, Intent, MenuScope};
use ui::{ContextMenu, Icon, IconName, IconWeight, Palette, Surface, Theme, radius};

use crate::panels::chats::RowInput;
use crate::panels::chats::input_view;
use crate::panels::chats::{ringed, row_focus};
use crate::panels::explorer::file_drop;
use crate::panels::explorer::file_menu::file_menu;
use crate::panels::explorer::file_tree_entry::FileTreeEntry;
use crate::panels::explorer::file_tree_row::badges::{
    count_gap, counts_view, labelled, letter_badge, status_color,
};
pub use crate::panels::explorer::file_tree_row::file_glyph::FileGlyph;
pub(crate) use crate::panels::explorer::file_tree_row::glyph::glyph;
pub(crate) use crate::panels::explorer::file_tree_row::tree_guides::tree_guides;
use crate::panels::explorer::search::highlight;
use crate::sidebar::{ICON_MEDIUM, ICON_TINY, SPACE_SM, TEXT_BODY};
use crate::sidebar::{RowAction, RowHandler};

const BASE_PADDING: f32 = 6.0;
const IGNORED_OPACITY: f32 = 0.5;
const MATCH_TINT: f32 = 0.07;
const HIGHLIGHT_TINT: f32 = 0.3;

#[derive(IntoElement)]
pub(crate) struct FileTreeRow {
    pub(crate) entry: FileTreeEntry,
    pub(crate) neighbours: (u8, u8),
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
        let letter = entry.status.letter().map(|l| letter_badge(l, mono));
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
        let guides = tree_guides((entry.depth, self.neighbours), (&self.look, palette));
        let row = ringed(tinted, &focus, palette)
            .children(guides)
            .child(chevron(&entry, palette))
            .child(glyph(&entry, (&self.look, palette), cx))
            .child(labelled(name, letter))
            .children(trailing_counts(&entry, (mono, &self.look), palette))
            .on_drop(file_drop::on_drop(
                &self.handler,
                file_drop::drop_directory(&entry),
            ))
            .on_click(select_then(&self.handler, entry.clone()));
        if self.input.is_some() {
            return row.into_any_element();
        }
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

fn name_cell(
    entry: &FileTreeEntry,
    input: Option<&RowInput>,
    look: (&'static str, &Palette),
) -> Div {
    match input {
        Some(input) => div().flex_1().min_w_0().child(input_view(input, look)),
        None => div()
            .debug_selector(|| "file-name".into())
            .min_w_0()
            .truncate()
            .child(entry.name.clone()),
    }
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
    div().min_w_0().truncate().child(text)
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

    #[gpui::test]
    fn a_row_is_twenty_six_pixels_tall(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| FileTreeRow {
            entry: entry(FileKind::File, "main.rs"),
            neighbours: (0, 0),
            input: None,
            selected: false,
            look: TreeLook::default(),
            scope: std::rc::Rc::default(),
            handler: std::rc::Rc::new(|_, _, _| {}),
        });
        assert_eq!(f32::from(bounds.size.height), 26.0);
    }

    #[gpui::test]
    fn the_status_letter_follows_the_name_and_the_counts_close_the_row(cx: &mut TestAppContext) {
        let changed = FileTreeEntry {
            status: GitStatus::Modified,
            counts: Some((3, 1)),
            ..entry(FileKind::File, "a.rs")
        };
        let cx = mount(cx, move |_, _| FileTreeRow {
            entry: changed.clone(),
            neighbours: (0, 0),
            input: None,
            selected: false,
            look: TreeLook::default(),
            scope: std::rc::Rc::default(),
            handler: std::rc::Rc::new(|_, _, _| {}),
        });
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
