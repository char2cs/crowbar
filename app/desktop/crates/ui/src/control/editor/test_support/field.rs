//! A focused text field in a test window.

use std::cell::Cell;
use std::rc::Rc;

use gpui::{Entity, IntoElement, Render, TestAppContext, VisualTestContext};

use crate::control::editor::test_support::drawn::Drawn;
use crate::control::editor::text_input::TextInput;
use crate::foundation::Motion;
use crate::foundation::Theme;

struct Counted {
    input: Entity<TextInput>,
    draws: Rc<Cell<usize>>,
}

impl Render for Counted {
    fn render(
        &mut self,
        _: &mut gpui::Window,
        _: &mut gpui::Context<'_, Self>,
    ) -> impl IntoElement {
        self.draws.set(self.draws.get() + 1);
        self.input.clone()
    }
}

fn install(cx: &mut TestAppContext) {
    cx.update(|cx| {
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
        cx.bind_keys(crate::control::editor::text_input::bindings());
    });
}

/// Opens a window holding a focused [`TextInput`] with the default bindings installed.
pub(crate) fn field(cx: &mut TestAppContext) -> (Entity<TextInput>, &mut VisualTestContext) {
    install(cx);
    let (input, cx) = cx.add_window_view(|_, cx| TextInput::new("Search", cx));
    cx.update(|window, cx| input.read(cx).focus(window));
    (input, cx)
}

/// Like [`field`], in the active window, with the number of times the window was drawn.
pub(crate) fn drawn_field(cx: &mut TestAppContext) -> Drawn<'_, TextInput> {
    install(cx);
    let draws = Rc::new(Cell::new(0));
    let counter = Rc::clone(&draws);
    let (host, cx) = cx.add_window_view(|_, cx| Counted {
        input: TextInput::build("Search", cx),
        draws: counter,
    });
    let input = host.read_with(cx, |host, _| host.input.clone());
    cx.update(|window, cx| {
        window.activate_window();
        input.read(cx).focus(window);
    });
    cx.run_until_parked();
    (input, draws, cx)
}

/// The text currently in `input`.
pub(crate) fn text_of(
    input: &Entity<TextInput>,
    cx: &mut VisualTestContext,
) -> String {
    input.read_with(cx, |input, _| input.text().to_owned())
}
