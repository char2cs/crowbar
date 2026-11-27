//! Bringing a line to the middle, over gpui-kit's public scroll and layout reads.

use gpui::Context;
use gpui_kit::base::input::{BufferPoint, RopeExt as _};
use gpui_kit::component::input::EditorState;

pub(crate) fn reveal_centered(
    state: &mut EditorState,
    offset: usize,
    cx: &mut Context<'_, EditorState>,
) {
    let Some(line_height) = state
        .line_height()
        .map(|height| f64::from(f32::from(height)))
    else {
        return;
    };
    let row = display_row(state, offset);
    let viewport = f64::from(f32::from(state.input_bounds().size.height));
    let centre = line_height * row - (viewport - line_height) / 2.0;
    let lowest = (viewport - state.content_height_f64()).min(0.0);
    state.set_scroll_y_f64((-centre).clamp(lowest, 0.0), cx);
}

fn display_row(
    state: &EditorState,
    offset: usize,
) -> f64 {
    let point = state.text().offset_to_point(offset);
    let buffer = BufferPoint::new(point.row, point.column);
    let row = state.display_map().buffer_pos_to_display_pos(buffer).row;
    u32::try_from(row).map_or(f64::from(u32::MAX), f64::from)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::Access;
    use crate::editor_view::test_support::{opened, state_of};

    const LINES: usize = 300;
    const TARGET: usize = 150;

    #[gpui::test]
    fn a_far_line_is_brought_to_the_middle(cx: &mut TestAppContext) {
        let text = (0..LINES).fold(String::new(), |text, line| text + &format!("line {line}\n"));
        let offset = text
            .match_indices('\n')
            .nth(TARGET - 1)
            .map_or(0, |(at, _)| at + 1);
        let (view, window) = opened((&text, None), Access::Editable, cx);
        window.run_until_parked();
        let state = window.read(|cx| state_of(&view, cx));
        window.update(|_, cx| state.update(cx, |state, cx| reveal_centered(state, offset, cx)));
        window.run_until_parked();
        let rows = window.read(|cx| state.read(cx).visible_row_range());
        let rows = rows.expect("rows are drawn");
        let middle = usize::midpoint(rows.start, rows.end);
        assert!(middle.abs_diff(TARGET) <= 2, "{rows:?}");
    }
}
