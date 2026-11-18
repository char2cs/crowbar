//! The text part of a tree row: title, optional branch line and line counts, or the inline input.

use gpui::prelude::FluentBuilder;
use gpui::{Div, FontWeight, ParentElement, Styled, div, px};
use sidebar::{Row, RowKind, Titling};
use ui::Palette;

use crate::row_input::RowInput;
use crate::scale::SPACE_XS;

const TITLE_SIZE: f32 = 13.0;
const SUBLABEL_SIZE: f32 = 10.5;
const SUBLABEL_LINE: f32 = 13.0;
// PLATFORM: Chrome places the baseline of these faces about a pixel lower than GPUI does.
const TITLE_NUDGE: f32 = 1.0;
const SUBLABEL_NUDGE: f32 = -0.5;

pub(crate) fn label(
    row: &Row,
    input: Option<&RowInput>,
    (mono, palette): (&'static str, &Palette),
) -> Div {
    let column = div()
        .flex()
        .flex_col()
        .flex_1()
        .min_w_0()
        .justify_center()
        .overflow_hidden();
    if let Some(input) = input {
        return column.child(input_view(input, (mono, palette)));
    }
    let title = title(row, mono);
    if !row.is_two_line() {
        return column.child(title);
    }
    column.child(title).child(sublabel(row, mono, palette))
}

pub(crate) fn input_view(
    input: &RowInput,
    (mono, palette): (&'static str, &Palette),
) -> Div {
    let line = div().flex().min_w_0().child(input.field.clone());
    match &input.error {
        Some(reason) => div()
            .flex()
            .flex_col()
            .child(line)
            .child(error_line(reason, mono, palette)),
        None => line,
    }
}

fn error_line(
    reason: &str,
    mono: &'static str,
    palette: &Palette,
) -> Div {
    div()
        .truncate()
        .font_family(mono)
        .text_size(px(SUBLABEL_SIZE))
        .line_height(px(SUBLABEL_LINE))
        .text_color(palette.destructive())
        .child(reason.to_owned())
}

fn title(
    row: &Row,
    mono: &'static str,
) -> Div {
    let title = div()
        .truncate()
        .text_size(px(TITLE_SIZE))
        .font_weight(FontWeight::MEDIUM)
        .relative()
        .top(px(TITLE_NUDGE))
        .child(row.label.clone());
    let title = title.when(row.titling == Titling::Provisional, Styled::italic);
    let monospaced =
        row.kind == RowKind::RepoHeader || (row.kind == RowKind::Branch && !row.is_two_line());
    if monospaced {
        return title.font_family(mono);
    }
    title
}

fn sublabel(
    row: &Row,
    mono: &'static str,
    palette: &Palette,
) -> Div {
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
        .gap(SPACE_XS)
        .truncate()
        .font_family(mono)
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
mod tests {
    use gpui::TestAppContext;
    use ui::{Surface, Theme};

    use super::*;
    use sidebar::{Workspace, WorkspaceStatus};

    use crate::test_support::{chat, measure};

    fn height(
        cx: &mut TestAppContext,
        row: Row,
    ) -> f32 {
        let bounds = measure(cx, move |_, cx| {
            let palette = Theme::of(cx).palette(Surface::Chrome);
            label(&row, None, (Theme::of(cx).mono_family(), palette))
        });
        f32::from(bounds.size.height)
    }

    #[gpui::test]
    fn a_row_with_a_workspace_shows_a_second_line(cx: &mut TestAppContext) {
        let plain = height(cx, chat("Refactor"));
        let mut two = Row::new("b", RowKind::Branch, 1, "Branch");
        two.workspace = Some(Workspace {
            status: WorkspaceStatus::New,
            branch: "feature".into(),
            added: 1,
            deleted: 0,
            held_by_path: None,
        });
        assert!(height(cx, two) > plain);
    }
}
