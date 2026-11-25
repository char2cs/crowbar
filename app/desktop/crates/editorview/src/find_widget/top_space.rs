use gpui::{Context, px};

use super::FindWidget;
use crate::reveal::Reveal;

pub(super) const FIND_HEIGHT: f32 = 33.0;
pub(super) const REPLACE_HEIGHT: f32 = 62.0;

impl FindWidget {
    pub(super) fn show(
        &mut self,
        reveal: Reveal,
        cx: &mut Context<'_, Self>,
    ) {
        self.reveal = reveal;
        let space = px(height(reveal));
        self.editor
            .update(cx, |state, cx| state.set_top_space(space, cx));
    }
}

// SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/editor/contrib/find/browser/findWidget.ts (find.addExtraSpaceOnTop: a view zone of the widget's height after line 0)
pub(super) const fn height(reveal: Reveal) -> f32 {
    match reveal {
        Reveal::Hidden => 0.0,
        Reveal::Find => FIND_HEIGHT,
        Reveal::Replace => REPLACE_HEIGHT,
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::FIND_HEIGHT;
    use crate::Access;
    use crate::editor_view::test_support::{find_of, on_find, opened, scroll_top_of};
    use crate::find_widget::FindWidget;
    use crate::reveal::Reveal;

    const LINES: usize = 200;

    #[gpui::test]
    fn a_match_on_the_first_line_shows_the_space_the_widget_opens_over_it(cx: &mut TestAppContext) {
        let mut lines = vec!["line"; LINES];
        lines[0] = "first";
        let (view, window) = opened((&lines.join("\n"), Some(1)), Access::Editable, cx);
        window.run_until_parked();
        let find = find_of(&view, window);
        on_find(&find, window, |find, window, cx| {
            find.open(Reveal::Find, window, cx);
            find.find_input
                .update(cx, |input, cx| input.set_text("first", cx));
            find.typed(cx);
        });
        window.run_until_parked();
        let open = f32::from(scroll_top_of(&view, window));
        on_find(&find, window, FindWidget::close);
        window.run_until_parked();
        let closed = f32::from(scroll_top_of(&view, window));
        assert_eq!((open, closed), (FIND_HEIGHT, 0.0));
    }
}
