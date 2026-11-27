use gpui::{App, ClickEvent, ElementId, Window};

use crate::control::Button;
use crate::control::ButtonSize;
use crate::control::ButtonVariant;
use crate::floating::dialog::dialog_style::DialogStyle;
use crate::foundation::CloseHandler;
use crate::foundation::IconName;

pub(crate) fn close_button(
    id: ElementId,
    on_close: Option<CloseHandler>,
    style: DialogStyle,
) -> Button {
    let size = match style {
        DialogStyle::App => ButtonSize::Sm,
        DialogStyle::Plain => ButtonSize::Lg,
    };
    let close = Button::icon_only((id, "close"), IconName::X)
        .variant(ButtonVariant::Ghost)
        .size(size);
    let Some(handler) = on_close else {
        return close;
    };
    close.on_click(move |event, window, cx| pointer_only(&handler, event, window, cx))
}

fn pointer_only(
    handler: &CloseHandler,
    event: &ClickEvent,
    window: &mut Window,
    cx: &mut App,
) {
    if event.is_keyboard() {
        return;
    }
    handler(window, cx);
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{Modifiers, TestAppContext};

    use super::*;
    use crate::test_support::probe::open;

    #[gpui::test]
    fn a_pointer_click_closes_and_a_key_on_the_focused_button_does_not(cx: &mut TestAppContext) {
        let closed = Rc::new(Cell::new(0));
        let focus = cx.update(|cx| cx.focus_handle());
        let (sink, held) = (Rc::clone(&closed), focus.clone());
        let window = open(cx, move |_, _| {
            let sink = Rc::clone(&sink);
            let handler: CloseHandler = Rc::new(move |_, _| sink.set(sink.get() + 1));
            close_button("d".into(), Some(handler), DialogStyle::Plain).focus_handle(held.clone())
        });
        window.update(|window, cx| window.focus(&focus, cx));
        window.simulate_keystrokes("enter space");
        assert_eq!(closed.get(), 0, "keys leave the dialog open");
        let button = window.debug_bounds("probe").unwrap_or_default().center();
        window.simulate_click(button, Modifiers::none());
        assert_eq!(closed.get(), 1);
    }
}
