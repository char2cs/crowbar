use gpui::{Div, Styled};

use crate::control::Button;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Fill {
    Content,
    Share,
}

impl Fill {
    pub(crate) fn apply(
        self,
        frame: Div,
    ) -> Div {
        match self {
            Self::Content => frame,
            Self::Share => frame.flex_1(),
        }
    }
}

impl Button {
    #[must_use]
    pub fn grow(mut self) -> Self {
        self.fill = Fill::Share;
        self
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{Modifiers, ParentElement, TestAppContext, div, point, px};

    use super::*;
    use crate::test_support::probe::open;

    #[gpui::test]
    fn growing_buttons_share_their_row_to_its_end(cx: &mut TestAppContext) {
        let clicks = Rc::new(Cell::new(0));
        let sink = Rc::clone(&clicks);
        let cx = open(cx, move |_, _| {
            let sink = Rc::clone(&sink);
            let last = Button::new("b", "B")
                .grow()
                .on_click(move |_, _, _| sink.set(sink.get() + 1));
            div()
                .flex()
                .w(px(200.0))
                .child(Button::new("a", "A").grow())
                .child(last)
        });
        cx.simulate_click(point(px(196.0), px(12.0)), Modifiers::none());
        assert_eq!(clicks.get(), 1);
    }
}
