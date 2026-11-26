//! The GitHub button of a repository's icon popover.

use std::rc::Rc;

use gpui::Context;
use gpui::prelude::FluentBuilder as _;
use ui::{Button, ButtonSize, ButtonVariant, IconName};

use super::{Act, IconEditor};

impl IconEditor {
    #[must_use]
    pub(crate) fn with_github(
        mut self,
        fetch: Act,
    ) -> Self {
        self.github = Some(fetch);
        self
    }

    pub(super) fn github_button(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<Button> {
        let fetch = Rc::clone(self.github.as_ref()?);
        let button = Button::new("icon-github", "GitHub")
            .icon(IconName::Star)
            .size(ButtonSize::Xs)
            .variant(ButtonVariant::Muted)
            .grow()
            .when(self.busy(cx), Button::disabled)
            .on_click(cx.listener(move |this, _, _, cx| {
                this.typing = false;
                fetch(cx);
                cx.notify();
            }));
        Some(button)
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;

    use gpui::TestAppContext;

    use super::*;

    #[gpui::test]
    fn the_github_button_ends_the_row_and_its_press_fetches_and_closes_the_field(
        cx: &mut TestAppContext
    ) {
        let fetched = Rc::new(Cell::new(0));
        let sink = Rc::clone(&fetched);
        let fetch: Act = Rc::new(move |_| sink.set(sink.get() + 1));
        cx.update(|cx| ui::init(cx).expect("ui installs"));
        let (editor, cx) = cx.add_window_view(|_, cx| {
            IconEditor::new((Rc::new(|_, _| {}), Rc::new(|_| false)), cx).with_github(fetch)
        });
        cx.run_until_parked();
        let row = cx.debug_bounds("icon-actions").unwrap_or_default();
        editor.update(cx, |this, _| this.typing = true);
        let end = gpui::point(row.right() - gpui::px(4.0), row.center().y);
        cx.simulate_click(end, gpui::Modifiers::none());
        assert_eq!(fetched.get(), 1, "GitHub is the row's last button");
        assert!(!editor.read_with(cx, |this, _| this.typing));
    }
}
