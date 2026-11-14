//! The text part of a tree row: title, optional branch line and line counts, or the inline input.

use gpui::{Div, FontWeight, ParentElement, Styled, div, px};
use ui::Palette;
use ui::font::MONO_FAMILY;

use crate::row::Row;
use crate::row_input::RowInput;
use crate::row_kind::RowKind;
use crate::titling::Titling;

const TITLE_SIZE: f32 = 13.0;
const SUBLABEL_SIZE: f32 = 10.5;
const SUBLABEL_LINE: f32 = 13.0;
// PLATFORM: Chrome places the baseline of these faces about a pixel lower than GPUI does.
const TITLE_NUDGE: f32 = 1.0;
const SUBLABEL_NUDGE: f32 = -0.5;

pub(crate) fn label(row: &Row, input: Option<&RowInput>, palette: &Palette) -> Div {
    let column = div()
        .flex()
        .flex_col()
        .flex_1()
        .min_w_0()
        .justify_center()
        .overflow_hidden();
    if let Some(input) = input {
        return column.child(input_view(input, palette));
    }
    let title = title(row);
    if !row.is_two_line() {
        return column.child(title);
    }
    column.child(title).child(sublabel(row, palette))
}

fn input_view(input: &RowInput, palette: &Palette) -> Div {
    let line = div().flex().min_w_0().child(input.field.clone());
    match &input.error {
        Some(reason) => div()
            .flex()
            .flex_col()
            .child(line)
            .child(error_line(reason, palette)),
        None => line,
    }
}

fn error_line(reason: &str, palette: &Palette) -> Div {
    div()
        .truncate()
        .font_family(MONO_FAMILY)
        .text_size(px(SUBLABEL_SIZE))
        .line_height(px(SUBLABEL_LINE))
        .text_color(palette.destructive())
        .child(reason.to_owned())
}

fn title(row: &Row) -> Div {
    let title = div()
        .truncate()
        .text_size(px(TITLE_SIZE))
        .font_weight(FontWeight::MEDIUM)
        .relative()
        .top(px(TITLE_NUDGE))
        .child(row.label.clone());
    let title = if row.titling == Titling::Provisional {
        title.italic()
    } else {
        title
    };
    let mono =
        row.kind == RowKind::RepoHeader || (row.kind == RowKind::Branch && !row.is_two_line());
    if mono {
        return title.font_family(MONO_FAMILY);
    }
    title
}

fn sublabel(row: &Row, palette: &Palette) -> Div {
    let counts = row.workspace.iter().flat_map(|workspace| {
        [
            (workspace.added, '+', palette.sidebar_count_added()),
            (workspace.deleted, '-', palette.sidebar_count_deleted()),
        ]
    });
    let branch = row
        .workspace
        .as_ref()
        .map_or_default(|workspace| workspace.branch.clone());
    let line = div()
        .flex()
        .gap(px(4.0))
        .truncate()
        .font_family(MONO_FAMILY)
        .text_size(px(SUBLABEL_SIZE))
        .line_height(px(SUBLABEL_LINE))
        .relative()
        .top(px(SUBLABEL_NUDGE))
        .text_color(palette.muted_foreground())
        .child(branch);
    counts
        .filter(|(count, ..)| *count > 0)
        .fold(line, |line, (count, sign, color)| {
            line.child(div().text_color(color).child(format!("{sign}{count}")))
        })
}

#[cfg(test)]
#[path = "row_label_tests.rs"]
mod tests;
