//! The selections an editor command acts on.

use std::ops::Range;

use gpui_kit::component::input::EditorState;

pub(super) fn of(state: &EditorState) -> Vec<Range<usize>> {
    vec![state.selected_range()]
}
