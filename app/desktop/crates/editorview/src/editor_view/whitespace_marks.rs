//! The whitespace markers of Monaco's `boundary` and `trailing` modes, which gpui-kit's editor
//! cannot pick: painted over the visible lines after the editor, as its own `all` markers look.

use editor::WhitespaceMode;
use gpui::{
    App, Bounds, ContentMask, Entity, IntoElement, Pixels, ShapedLine, SharedString, Styled as _,
    TextAlign, TextRun, Window, canvas, point,
};
use gpui_kit::base::input::RopeExt as _;
use gpui_kit::component::input::EditorState;

const SPACE: &str = "•";
const TAB: &str = "→";

pub(super) fn layer(
    state: &Entity<EditorState>,
    mode: WhitespaceMode,
) -> impl IntoElement {
    let state = state.clone();
    canvas(
        |_, _, _| (),
        move |_, (), window, cx| paint(&state, mode, window, cx),
    )
    .absolute()
    .size_full()
}

fn paint(
    state: &Entity<EditorState>,
    mode: WhitespaceMode,
    window: &mut Window,
    cx: &mut App,
) {
    if matches!(mode, WhitespaceMode::None | WhitespaceMode::All) {
        return;
    }
    let state = state.read(cx);
    let (Some(rows), Some(area)) = (state.visible_row_range(), state.text_bounds()) else {
        return;
    };
    let display = state.display_map();
    let first = display.display_row_to_buffer_line(rows.start);
    let last = display.display_row_to_buffer_line(rows.end.saturating_sub(1));
    let marks: Vec<_> = (first..=last)
        .flat_map(|line| marks_of(state, mode, line))
        .collect();
    let start = state.text().line_start_offset(first);
    let scrolled = state
        .range_to_bounds(&(start..start))
        .map(|bounds| bounds.left());
    let left = scrolled.map_or(area.left(), |left| left - state.scroll_offset().x);
    let mask = Bounds::from_corners(point(left, area.top()), area.bottom_right());
    window.with_content_mask(Some(ContentMask { bounds: mask }), |window| {
        paint_marks(marks, window, cx);
    });
}

fn paint_marks(
    marks: Vec<(Bounds<Pixels>, char)>,
    window: &mut Window,
    cx: &mut App,
) {
    for mark in marks {
        paint_mark(mark, window, cx);
    }
}

fn marks_of(
    state: &EditorState,
    mode: WhitespaceMode,
    line: usize,
) -> Vec<(Bounds<Pixels>, char)> {
    let text = state.text();
    let start = text.line_start_offset(line);
    let content = text.slice_line(line).to_string();
    editor::marked_whitespace(mode, &content)
        .filter_map(|(offset, blank)| {
            let at = start + offset;
            state
                .range_to_bounds(&(at..at + 1))
                .map(|bounds| (bounds, blank))
        })
        .collect()
}

fn paint_mark(
    (bounds, blank): (Bounds<Pixels>, char),
    window: &mut Window,
    cx: &mut App,
) {
    let shaped = shaped_mark(blank, window, cx);
    let x = match blank {
        '\t' => bounds.left(),
        _ => bounds.center().x - shaped.width / 2.,
    };
    let origin = point(x, bounds.top());
    if let Err(error) = shaped.paint(
        origin,
        bounds.size.height,
        TextAlign::Left,
        None,
        window,
        cx,
    ) {
        tracing::warn!("whitespace mark: {error}");
    }
}

fn shaped_mark(
    blank: char,
    window: &Window,
    cx: &App,
) -> ShapedLine {
    let style = window.text_style();
    let size = style.font_size.to_pixels(window.rem_size());
    let (mark, size) = match blank {
        '\t' => (TAB, size),
        _ => (SPACE, size / 2.),
    };
    let run = TextRun {
        len: mark.len(),
        font: style.font(),
        color: gpui_kit::component::Theme::global(cx).muted_foreground,
        background_color: None,
        underline: None,
        strikethrough: None,
    };
    window
        .text_system()
        .shape_line(SharedString::new_static(mark), size, &[run], None)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::Access;
    use crate::editor_view::test_support::opened;

    #[gpui::test]
    fn each_mode_marks_the_spaces_monaco_does_on_a_shown_line(cx: &mut TestAppContext) {
        let cases = [
            (WhitespaceMode::None, 0),
            (WhitespaceMode::Trailing, 2),
            (WhitespaceMode::Boundary, 4),
            (WhitespaceMode::All, 5),
        ];
        let (view, window) = opened(("a b  c  \nx", None), Access::Editable, cx);
        window.run_until_parked();
        for (mode, count) in cases {
            let marks = window.read(|cx| marks_of(view.read(cx).state.read(cx), mode, 0).len());
            assert_eq!(marks, count, "{mode:?}");
        }
    }
}
