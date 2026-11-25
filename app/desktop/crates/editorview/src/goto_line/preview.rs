use gpui::{Context, Hsla};
use gpui_kit::component::input::{RangeDecoration, RangeDecorationStyle};
use ui::{Appearance, Theme};

use super::GotoLine;

// SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/editor/common/core/editorColorRegistry.ts (editorRangeHighlight)
const DARK_LINE_HIGHLIGHT: u32 = 0xffff_ff0b;
const LIGHT_LINE_HIGHLIGHT: u32 = 0xfdff_0033;

impl GotoLine {
    pub(super) fn preview(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let text = self.editor.read(cx).value();
        let Some(offset) = self.target(cx).offset_in(&text) else {
            self.highlight.clear(cx);
            return cx.notify();
        };
        let line_start = text
            .get(..offset)
            .and_then(|head| head.rfind('\n'))
            .map_or(0, |at| at + 1);
        let line_end = text
            .get(offset..)
            .and_then(|tail| tail.find('\n'))
            .map_or(text.len(), |at| offset + at);
        let fill = line_highlight(cx);
        let decoration = RangeDecoration::new(line_start..line_end.max(line_start + 1))
            .with_style(RangeDecorationStyle::Fill)
            .with_color(fill);
        self.highlight.set(vec![decoration], cx);
        self.editor.update(cx, |state, cx| {
            crate::reveal::centring::reveal_centered(state, offset, cx);
        });
        cx.notify();
    }
}

fn line_highlight(cx: &Context<'_, GotoLine>) -> Hsla {
    let rgba = match Theme::of(cx).appearance() {
        Appearance::Dark => DARK_LINE_HIGHLIGHT,
        Appearance::Light => LIGHT_LINE_HIGHLIGHT,
    };
    Hsla::from(gpui::rgba(rgba))
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::Access;
    use crate::editor_view::test_support::{opened, rows_of};

    #[gpui::test]
    fn typing_a_line_shows_it_without_moving_the_caret(cx: &mut TestAppContext) {
        let text = "line\n".repeat(200);
        let cases = [("120", 1, true), ("", 1, false)];
        for (typed, caret, scrolled) in cases {
            let (view, window) = opened((&text, Some(1)), Access::Editable, cx);
            window.run_until_parked();
            window.simulate_keystrokes("ctrl-g");
            window.simulate_input(typed);
            window.run_until_parked();
            let line = view.read_with(window, |view, cx| view.caret(cx).line);
            let rows = rows_of(&view, window);
            let moved = rows.is_some_and(|rows| rows.start > 0);
            assert_eq!((line, moved), (caret, scrolled), "{typed}");
        }
    }
}
