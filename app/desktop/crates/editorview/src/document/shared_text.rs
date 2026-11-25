//! The text of one document, which every view over it shows: a view's edit lands here and the
//! other views replay it into their own editor, each keeping its own carets and undo history.

use gpui::{Context, EventEmitter};
use gpui_kit::base::input::Rope;

pub(crate) struct SharedText {
    text: Rope,
}

impl SharedText {
    pub(crate) fn new(text: &str) -> Self {
        Self {
            text: Rope::from(text),
        }
    }

    pub(crate) const fn text(&self) -> &Rope {
        &self.text
    }

    pub(crate) fn set(
        &mut self,
        text: Rope,
        cx: &mut Context<'_, Self>,
    ) {
        if self.text == text {
            return;
        }
        self.text = text;
        cx.emit(self.text.clone());
    }
}

impl EventEmitter<Rope> for SharedText {}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};

    use super::*;

    #[gpui::test]
    fn only_a_change_of_text_is_announced(cx: &mut TestAppContext) {
        let shared = cx.new(|_| SharedText::new("a"));
        let heard = std::rc::Rc::new(std::cell::Cell::new(0));
        let count = std::rc::Rc::clone(&heard);
        cx.update(|cx| {
            cx.subscribe(&shared, move |_, _: &Rope, _| count.set(count.get() + 1))
                .detach();
        });
        for text in ["a", "b", "b"] {
            shared.update(cx, |shared, cx| shared.set(Rope::from(text), cx));
        }
        cx.run_until_parked();
        assert_eq!(heard.get(), 1);
    }
}
