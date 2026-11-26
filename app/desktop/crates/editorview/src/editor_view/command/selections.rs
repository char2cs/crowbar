//! The selections an editor command acts on.

use std::ops::Range;

use gpui_kit::component::input::EditorState;

pub(in crate::editor_view) fn of(state: &EditorState) -> Vec<Range<usize>> {
    vec![state.selected_range()]
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::Access;
    use crate::editor_view::test_support::{opened, state_of};

    #[gpui::test]
    fn a_command_acts_on_the_active_selection(cx: &mut TestAppContext) {
        let (view, window) = opened(("one\ntwo\n", None), Access::Editable, cx);
        let state = window.read(|cx| state_of(&view, cx));
        window.update(|_, cx| state.update(cx, |state, cx| state.set_selected_range(4..7, cx)));
        let acted = window.read(|cx| of(state.read(cx)));
        assert_eq!(acted, vec![4..7]);
    }
}
