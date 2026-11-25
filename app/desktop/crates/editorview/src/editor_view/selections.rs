//! The selections an editor command acts on.

use std::ops::Range;

use gpui_kit::component::input::EditorState;

/// The active selection: gpui-kit lets its owner read no other, so the commands of a view with
/// several carets act on the active one, as the owner accepted.
pub(super) fn of(state: &EditorState) -> Vec<Range<usize>> {
    vec![state.selected_range()]
}
