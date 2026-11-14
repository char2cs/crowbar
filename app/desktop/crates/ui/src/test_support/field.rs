//! A focused text field in a test window.

use gpui::{Entity, TestAppContext, VisualTestContext};

use crate::motion::Motion;
use crate::text_input::TextInput;
use crate::theme::Theme;

/// Opens a window holding a focused [`TextInput`] with the default bindings installed.
pub(crate) fn field(cx: &mut TestAppContext) -> (Entity<TextInput>, &mut VisualTestContext) {
    cx.update(|cx| {
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
        cx.bind_keys(crate::text_input_actions::bindings());
    });
    let (input, cx) = cx.add_window_view(|_, cx| TextInput::new("Search", cx));
    cx.update(|window, cx| input.read(cx).focus(window));
    (input, cx)
}

/// The text currently in `input`.
pub(crate) fn text_of(input: &Entity<TextInput>, cx: &mut VisualTestContext) -> String {
    input.read_with(cx, |input, _| input.text().to_owned())
}
