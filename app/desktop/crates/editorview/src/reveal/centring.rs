//! Monaco's centred reveal of a line, over gpui-kit's public scroll and layout reads.

use gpui::Context;
use gpui_kit::base::input::{BufferPoint, RopeExt as _};
use gpui_kit::component::input::EditorState;

/// Scrolls so the line holding `offset` sits in the middle of the viewport, as Monaco's
/// `revealRangeInCenter`; does nothing before the editor's first layout.
pub(crate) fn reveal_centered(
    state: &mut EditorState,
    offset: usize,
    cx: &mut Context<'_, EditorState>,
) {
    let Some(line_height) = state.line_height().map(|height| f64::from(f32::from(height))) else {
        return;
    };
    let row = display_row(state, offset);
    let viewport = f64::from(f32::from(state.input_bounds().size.height));
    let centre = line_height * row - (viewport - line_height) / 2.0;
    let lowest = (viewport - state.content_height_f64()).min(0.0);
    state.set_scroll_y_f64((-centre).clamp(lowest, 0.0), cx);
}

/// Whether the line holding `offset` shows in the viewport with `context` lines above it and one
/// fewer below, at most half the viewport each: the test Monaco's
/// `revealRangeInCenterIfOutsideViewport` makes before it scrolls. `None` before the first
/// layout.
pub(crate) fn shows_line_with_context(
    state: &EditorState,
    offset: usize,
    context: u8,
) -> Option<bool> {
    let line_height = f64::from(f32::from(state.line_height()?));
    let row = display_row(state, offset);
    let viewport = f64::from(f32::from(state.input_bounds().size.height));
    let above = (line_height * f64::from(context)).min(viewport / 2.0);
    let below = (above - line_height).max(0.0);
    let top = line_height * row - above;
    let bottom = line_height * (row + 1.0) + below;
    let scroll = -state.scroll_y_f64();
    Some(top >= scroll && bottom <= scroll + viewport)
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
