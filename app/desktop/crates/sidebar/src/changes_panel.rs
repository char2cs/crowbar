//! The Changes panel: branch pill, file count and the changed files.

use gpui::{AnyElement, Context, Div, IntoElement, ParentElement, Pixels, Styled, div, px};
use ui::font::MONO_FAMILY;
use ui::{Icon, IconName, Palette, ScrollArea, Surface, Theme, radius, shadow};

use crate::changed_file::ChangedFile;
use crate::changes::Changes;
use crate::file_kind::FileKind;
use crate::file_tree_entry::FileTreeEntry;
use crate::file_tree_row::FileTreeRow;
use crate::git_status::GitStatus;
use crate::panel_message::panel_message;
use crate::row_action::RowHandler;
use crate::scale::{ICON_MEDIUM, SPACE_MD, SPACE_SM, SPACE_XS, TEXT_BODY, TEXT_CAPTION};
use crate::sidebar::Sidebar;

const PILL_HEIGHT: Pixels = px(36.0);
const PILL_SHADOW: f32 = 0.1;
const COUNT_INSET: Pixels = px(14.0);

fn entry(file: ChangedFile) -> FileTreeEntry {
    let name = file
        .path
        .rsplit('/')
        .next()
        .unwrap_or(&file.path)
        .to_owned();
    let changed = file.added + file.deleted > 0 && file.status != GitStatus::Clean;
    FileTreeEntry {
        counts: changed.then_some((file.added, file.deleted)),
        path: file.path,
        name,
        depth: 0,
        kind: FileKind::File,
        status: file.status,
    }
}

fn branch_pill(
    changes: &Changes,
    palette: &Palette,
) -> Div {
    let parent = changes.parent.clone().map(|parent| {
        div()
            .flex()
            .gap(SPACE_MD)
            .text_color(palette.control_muted_foreground())
            .child("→")
            .child(div().font_family(MONO_FAMILY).truncate().child(parent))
    });
    div()
        .mx(SPACE_SM)
        .my(px(2.0))
        .flex()
        .items_center()
        .gap(SPACE_MD)
        .h(PILL_HEIGHT)
        .px(SPACE_MD)
        .rounded(radius::LG)
        .bg(palette.control_surface())
        .text_color(palette.control_foreground())
        .text_size(TEXT_BODY)
        .shadow(shadow::xs(PILL_SHADOW))
        .child(
            Icon::new(IconName::GitBranch)
                .size(ICON_MEDIUM)
                .color(palette.control_muted_foreground()),
        )
        .child(
            div()
                .font_family(MONO_FAMILY)
                .truncate()
                .child(changes.branch.clone()),
        )
        .children(parent)
}

fn count_line(
    count: usize,
    palette: &Palette,
) -> Div {
    let plural = match count {
        1 => "",
        _ => "s",
    };
    div()
        .px(COUNT_INSET)
        .pt(SPACE_MD)
        .pb(SPACE_XS)
        .text_size(TEXT_CAPTION)
        .text_color(palette.muted_foreground())
        .child(format!("Changed — {count} file{plural}"))
}

fn rows(
    files: Vec<ChangedFile>,
    handler: &RowHandler,
) -> impl Iterator<Item = FileTreeRow> + '_ {
    files.into_iter().map(|file| FileTreeRow {
        entry: entry(file),
        handler: std::rc::Rc::clone(handler),
    })
}

impl Sidebar {
    pub(crate) fn changes_panel(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let changes = self.data.changes();
        if changes.branch.is_empty() {
            return panel_message("No repository open", cx);
        }
        let handler = Self::row_handler(cx);
        let list = div()
            .flex()
            .flex_col()
            .child(branch_pill(&changes, palette))
            .child(count_line(changes.files.len(), palette))
            .children(rows(changes.files, &handler));
        ScrollArea::new("changes-scroll", &self.scrolls.changes, list).into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn the_changes_panel_renders_the_changed_files(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Changes, cx));
        cx.run_until_parked();
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.data.changes().files.len()),
            1
        );
    }

    #[test]
    fn a_changed_file_row_carries_its_line_counts() {
        use crate::changed_file::ChangedFile;
        use crate::git_status::GitStatus;
        let file = ChangedFile {
            path: "a/b.rs".into(),
            status: GitStatus::Added,
            added: 3,
            deleted: 1,
        };
        assert_eq!(entry(file).counts, Some((3, 1)));
    }
}
