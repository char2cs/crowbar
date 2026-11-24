use std::ops::Range;

use editor::GotoTarget;
use gpui::{
    AppContext as _, Context, Entity, Focusable as _, InteractiveElement as _, IntoElement,
    ParentElement as _, Pixels, Point, Render, Styled as _, Subscription, Window, div, px,
};
use gpui_kit::component::input::{EditorState, RangeDecorationCollection};
use ui::{Surface, TextInput, TextInputEvent, TextInputStyle, Theme, shadow};

mod preview;

const PREFIX: &str = ":";

type ViewBefore = (Range<usize>, Point<Pixels>);
// SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/platform/quickinput/browser/media/quickInput.css
const MAX_WIDTH: f32 = 600.0;
const WIDTH_SHARE: f32 = 0.62;
const PADDING: f32 = 6.0;
const INPUT_HEIGHT: f32 = 26.0;
const INPUT_PADDING_X: f32 = 6.0;
const ROW_HEIGHT: f32 = 22.0;
const ROW_PADDING_X: f32 = 11.0;
const LABEL_SIZE: f32 = 13.0;
const RADIUS: f32 = 6.0;
const INPUT_RADIUS: f32 = 2.0;
const TOP: f32 = 8.0;
const SHADOW_ALPHA: f32 = 0.36;

pub(crate) struct GotoLine {
    editor: Entity<EditorState>,
    input: Entity<TextInput>,
    shown: bool,
    before: Option<ViewBefore>,
    width: Pixels,
    highlight: RangeDecorationCollection,
    _subscription: Subscription,
}

impl GotoLine {
    pub(crate) fn new(
        editor: Entity<EditorState>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let input = cx.new(|cx| TextInput::new("", cx).style(TextInputStyle::Bare));
        let highlight = editor.update(cx, |state, cx| {
            state.create_range_decorations_collection(Vec::new(), cx)
        });
        let subscription = cx.subscribe_in(&input, window, |goto, _, event, window, cx| {
            goto.on_input(event, window, cx);
        });
        Self {
            editor,
            input,
            shown: false,
            before: None,
            width: px(MAX_WIDTH),
            highlight,
            _subscription: subscription,
        }
    }

    pub(crate) fn open(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let state = self.editor.read(cx);
        self.before = Some((state.selected_range(), state.scroll_offset()));
        self.width = (state.input_bounds().size.width * WIDTH_SHARE).min(px(MAX_WIDTH));
        self.shown = true;
        self.input
            .update(cx, |input, cx| input.set_text(PREFIX, cx));
        let focus = self.input.read(cx).focus_handle(cx);
        window.focus(&focus, cx);
        cx.notify();
    }

    fn target(
        &self,
        cx: &Context<'_, Self>,
    ) -> GotoTarget {
        let text = self.editor.read(cx).value();
        GotoTarget::parse(self.input.read(cx).text(), &text)
    }

    fn on_input(
        &mut self,
        event: &TextInputEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            TextInputEvent::Changed(_) => self.preview(cx),
            TextInputEvent::Submitted => self.go(window, cx),
            TextInputEvent::Cancelled => self.cancel(window, cx),
            _ => {},
        }
    }

    fn go(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let text = self.editor.read(cx).value();
        let Some(offset) = self.target(cx).offset_in(&text) else {
            return;
        };
        self.editor.update(cx, |state, cx| {
            state.set_selected_range(offset..offset, cx);
            state.reveal_centered(offset, cx);
        });
        self.hide(window, cx);
    }

    fn cancel(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(before) = self.before.take() {
            self.restore(before, cx);
        }
        self.hide(window, cx);
    }

    fn restore(
        &self,
        (selected, scroll): ViewBefore,
        cx: &mut Context<'_, Self>,
    ) {
        self.editor.update(cx, |state, cx| {
            state.set_selected_range(selected, cx);
            state.set_scroll_offset(scroll, cx);
        });
    }

    fn hide(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.shown = false;
        self.before = None;
        self.highlight.clear(cx);
        let focus = self.editor.read(cx).focus_handle(cx);
        window.focus(&focus, cx);
        cx.notify();
    }
}

impl Render for GotoLine {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        if !self.shown {
            return div().into_any_element();
        }
        let palette = *Theme::of(cx).palette(Surface::Window);
        let label = self.target(cx).label().to_owned();
        div()
            .absolute()
            .top(px(TOP))
            .left_0()
            .right_0()
            .flex()
            .justify_center()
            .child(
                div()
                    .id("goto-line")
                    .w(self.width)
                    .p(px(PADDING))
                    .flex()
                    .flex_col()
                    .rounded(px(RADIUS))
                    .bg(palette.background())
                    .text_color(palette.foreground())
                    .shadow(shadow::md(SHADOW_ALPHA))
                    .on_mouse_down(gpui::MouseButton::Left, |_, _, cx| cx.stop_propagation())
                    .child(self.input_box(&palette))
                    .child(label_row(label)),
            )
            .into_any_element()
    }
}

impl GotoLine {
    fn input_box(
        &self,
        palette: &ui::Palette,
    ) -> gpui::Div {
        div()
            .h(px(INPUT_HEIGHT))
            .px(px(INPUT_PADDING_X))
            .flex()
            .items_center()
            .border_1()
            .border_color(palette.border())
            .rounded(px(INPUT_RADIUS))
            .child(self.input.clone())
    }
}

fn label_row(label: String) -> gpui::Div {
    div()
        .h(px(ROW_HEIGHT))
        .px(px(ROW_PADDING_X))
        .flex()
        .items_center()
        .text_size(px(LABEL_SIZE))
        .child(label)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::Access;
    use crate::editor_view::test_support::{opened, rows_of};

    const LINES: usize = 200;
    const MIDDLE_SLACK: usize = 2;

    #[gpui::test]
    fn ctrl_g_goes_to_the_typed_line_centred_and_escape_puts_the_view_back(
        cx: &mut TestAppContext
    ) {
        let text = "line\n".repeat(LINES);
        let cases = [
            ("120", "enter", 120, Some(119)),
            ("120:3", "enter", 120, Some(119)),
            ("120", "escape", 1, None),
        ];
        for (typed, last, line, centred) in cases {
            let (view, window) = opened((&text, Some(1)), Access::Editable, cx);
            window.run_until_parked();
            window.simulate_keystrokes("ctrl-g");
            window.simulate_input(typed);
            window.simulate_keystrokes(last);
            window.run_until_parked();
            let caret = view.read_with(window, |view, cx| view.caret(cx).line);
            assert_eq!(caret, line, "{typed} {last}");
            let rows = rows_of(&view, window);
            let rows = rows.unwrap_or_default();
            let placed = placed(&rows, centred);
            assert!(placed, "{typed} {last}: rows {rows:?}");
        }
    }

    fn placed(
        rows: &std::ops::Range<usize>,
        centred: Option<usize>,
    ) -> bool {
        let Some(row) = centred else {
            return rows.start == 0;
        };
        usize::midpoint(rows.start, rows.end).abs_diff(row) <= MIDDLE_SLACK
    }
}
