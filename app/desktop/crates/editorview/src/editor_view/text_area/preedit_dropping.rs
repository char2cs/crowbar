//! The editor's text input as gpui-kit's, except that a press drops a composition still in
//! progress, as develop's web view does through GTK's input method context.

use std::ops::Range;

use gpui::{
    App, Bounds, ElementInputHandler, Entity, Focusable as _, InputHandler, IntoElement, Pixels,
    Point, Styled as _, UTF16Selection, Window, canvas,
};
use gpui_kit::component::input::EditorState;

// PLATFORM: linux only; on macos stock unmarking commits the preedit, as WebKit does there.
pub(in crate::editor_view) fn layer(state: &Entity<EditorState>) -> impl IntoElement {
    let state = state.clone();
    canvas(
        |_, _, _| (),
        move |bounds, (), window, cx| {
            let editor = state.read(cx);
            let focus = editor.focus_handle(cx);
            let bounds = editor.text_bounds().unwrap_or(bounds);
            let handler = PreeditDropping::new(bounds, state.clone());
            window.handle_input(&focus, handler, cx);
        },
    )
    .absolute()
    .size_full()
}

struct PreeditDropping {
    inner: ElementInputHandler<EditorState>,
}

impl PreeditDropping {
    fn new(
        bounds: Bounds<Pixels>,
        state: Entity<EditorState>,
    ) -> Self {
        Self {
            inner: ElementInputHandler::new(bounds, state),
        }
    }
}

impl InputHandler for PreeditDropping {
    fn unmark_text(
        &mut self,
        window: &mut Window,
        cx: &mut App,
    ) {
        if let Some(marked) = self.inner.marked_text_range(window, cx) {
            self.inner
                .replace_text_in_range(Some(marked), "", window, cx);
        }
        self.inner.unmark_text(window, cx);
    }

    fn selected_text_range(
        &mut self,
        ignore_disabled_input: bool,
        window: &mut Window,
        cx: &mut App,
    ) -> Option<UTF16Selection> {
        self.inner
            .selected_text_range(ignore_disabled_input, window, cx)
    }

    fn marked_text_range(
        &mut self,
        window: &mut Window,
        cx: &mut App,
    ) -> Option<Range<usize>> {
        self.inner.marked_text_range(window, cx)
    }

    fn text_for_range(
        &mut self,
        range_utf16: Range<usize>,
        adjusted_range: &mut Option<Range<usize>>,
        window: &mut Window,
        cx: &mut App,
    ) -> Option<String> {
        self.inner
            .text_for_range(range_utf16, adjusted_range, window, cx)
    }

    fn replace_text_in_range(
        &mut self,
        replacement_range: Option<Range<usize>>,
        text: &str,
        window: &mut Window,
        cx: &mut App,
    ) {
        self.inner
            .replace_text_in_range(replacement_range, text, window, cx);
    }

    fn replace_and_mark_text_in_range(
        &mut self,
        range_utf16: Option<Range<usize>>,
        new_text: &str,
        new_selected_range: Option<Range<usize>>,
        window: &mut Window,
        cx: &mut App,
    ) {
        self.inner.replace_and_mark_text_in_range(
            range_utf16,
            new_text,
            new_selected_range,
            window,
            cx,
        );
    }

    fn bounds_for_range(
        &mut self,
        range_utf16: Range<usize>,
        window: &mut Window,
        cx: &mut App,
    ) -> Option<Bounds<Pixels>> {
        self.inner.bounds_for_range(range_utf16, window, cx)
    }

    fn character_index_for_point(
        &mut self,
        point: Point<Pixels>,
        window: &mut Window,
        cx: &mut App,
    ) -> Option<usize> {
        self.inner.character_index_for_point(point, window, cx)
    }

    fn element_bounds(
        &mut self,
        window: &mut Window,
        cx: &mut App,
    ) -> Option<Bounds<Pixels>> {
        self.inner.element_bounds(window, cx)
    }

    fn prefers_ime_for_printable_keys(
        &mut self,
        window: &mut Window,
        cx: &mut App,
    ) -> bool {
        self.inner.prefers_ime_for_printable_keys(window, cx)
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::editor_view::test_support::opened;
    use crate::{Access, EditorView};

    #[gpui::test]
    fn a_press_drops_the_composition_and_keeps_the_typed_text(cx: &mut TestAppContext) {
        let (view, window) = opened(("ab", None), Access::Editable, cx);
        window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
        let state = window.read(|cx| view.read(cx).state.clone());
        window.update(|window, cx| {
            let mut handler = PreeditDropping::new(Bounds::default(), state);
            handler.replace_and_mark_text_in_range(None, "ni", None, window, cx);
            handler.unmark_text(window, cx);
        });
        assert_eq!(view.read_with(window, EditorView::text), "ab");
    }
}
