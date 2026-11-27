mod changed_tree_row;
mod merge_card;
mod merge_draft;
mod merge_lines;
mod viewed_box;

pub(crate) use merge_draft::MergeDraft;

use gpui::{
    AnyElement, Context, Div, FontWeight, InteractiveElement, IntoElement, ParentElement, Pixels,
    Styled, div, px,
};
use sidebar::{ChangedRow, Changes, Focus, IndentGuides};
use ui::{Icon, IconName, Palette, ScrollArea, Surface, Theme, radius, shadow};

use crate::panels::changes::changed_tree_row::ChangedTreeRow;
use crate::panels::explorer::{TreeLook, neighbours};
use crate::panels::message::panel_message;
use crate::sidebar::Sidebar;
use crate::sidebar::{
    ICON_MEDIUM, SPACE_LG, SPACE_MD, SPACE_SM, SPACE_XS, TEXT_BODY, TEXT_CAPTION, TEXT_SMALL,
};

const PILL_HEIGHT: Pixels = px(36.0);
const PILL_SHADOW: f32 = 0.1;
const COUNT_INSET: Pixels = px(14.0);
const ROW_INDENT_STEP: f32 = 14.0;

fn branch_pill(
    changes: &Changes,
    mono: &'static str,
    palette: &Palette,
) -> Div {
    let parent = changes.parent.clone().map(|parent| {
        div()
            .flex()
            .gap(SPACE_MD)
            .text_color(palette.control_muted_foreground())
            .child("→")
            .child(div().font_family(mono).truncate().child(parent))
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
                .flex_1()
                .min_w_0()
                .font_family(mono)
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

const GIT_ERROR: &str = "changes-git-error";
const UNREADABLE: &str = "Git can't read this workspace";

fn git_error(
    reason: String,
    palette: &Palette,
) -> Div {
    div()
        .debug_selector(|| GIT_ERROR.to_owned())
        .mx(SPACE_LG)
        .my(SPACE_MD)
        .flex()
        .flex_col()
        .gap(SPACE_XS)
        .min_w_0()
        .child(
            div()
                .text_size(TEXT_SMALL)
                .font_weight(FontWeight::MEDIUM)
                .text_color(palette.foreground())
                .child(UNREADABLE),
        )
        .child(
            div()
                .text_size(TEXT_CAPTION)
                .text_color(palette.muted_foreground())
                .child(reason),
        )
}

impl Sidebar {
    fn changed_rows(
        &self,
        changes: &Changes,
        cx: &mut Context<'_, Self>,
    ) -> Vec<ChangedTreeRow> {
        let handler = Self::row_handler(cx);
        let look = TreeLook {
            indent: ROW_INDENT_STEP,
            guides: IndentGuides::Drawn,
            ..TreeLook::of(&self.data.file_tree_view(), None)
        };
        let rows = changes.rows(self.tree.files().changed_folds());
        let depths: Vec<u8> = rows.iter().map(ChangedRow::depth).collect();
        rows.into_iter()
            .enumerate()
            .map(|(at, row)| ChangedTreeRow {
                row,
                neighbours: neighbours(&depths, at),
                look: look.clone(),
                handler: std::rc::Rc::clone(&handler),
            })
            .collect()
    }

    pub(crate) fn changes_panel(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let changes = self.data.changes();
        if self.data.focus() != Focus::Repository {
            return panel_message("No repository open", cx);
        }
        let mono = Theme::of(cx).mono_family();
        let pill = (!changes.branch.is_empty()).then(|| branch_pill(&changes, mono, palette));
        if let Some(reason) = changes.error.clone() {
            let column = div()
                .flex()
                .flex_col()
                .children(pill)
                .child(git_error(reason, palette));
            return column.into_any_element();
        }
        let rows = self.changed_rows(&changes, cx);
        let list = div()
            .flex()
            .flex_col()
            .children(pill)
            .children(self.merge_card(cx))
            .child(count_line(changes.files.len(), palette))
            .children(rows);
        ScrollArea::new("changes-scroll", &self.scrolls.changes, list).into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn the_changes_panel_renders_the_changed_files(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Changes, cx));
        cx.run_until_parked();
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.data.changes().files.len()),
            1
        );
    }

    #[gpui::test]
    fn a_workspace_git_cannot_read_shows_the_error_instead_of_the_files(cx: &mut TestAppContext) {
        let data = DemoShell::reference().with_git_error("not a git repository");
        let (_, sidebar, cx) = open(cx, data);
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Changes, cx));
        cx.run_until_parked();
        assert!(cx.debug_bounds("changes-git-error").is_some(), "the error");
        assert!(cx.debug_bounds("changed-row-src").is_none(), "no tree");
    }

    #[gpui::test]
    fn changed_files_sit_under_folders_that_fold(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Changes, cx));
        cx.run_until_parked();
        assert!(
            cx.debug_bounds("changed-row-src").is_some(),
            "the folder row"
        );
        assert!(
            cx.debug_bounds("changed-row-src/lib.rs").is_some(),
            "the file row"
        );
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::ToggleChangedFold("src".into()), window, cx);
        });
        cx.run_until_parked();
        assert!(
            cx.debug_bounds("changed-row-src/lib.rs").is_none(),
            "folded away"
        );
    }
}
