//! The stand-in for the panes area.

use gpui::{Context, Hsla, IntoElement, ParentElement, Render, Styled, Window, div};

pub(crate) struct Content {
    pub(crate) background: Hsla,
}

impl Render for Content {
    fn render(
        &mut self,
        _window: &mut Window,
        _cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        div().size_full().bg(self.background).child("")
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use gpui::TestAppContext;

    #[gpui::test]
    fn the_stand_in_opens_in_a_window(cx: &mut TestAppContext) {
        let window = cx.add_window(|_, _| Content {
            background: gpui::black(),
        });
        cx.run_until_parked();
        assert!(window.update(cx, |_, _, _| ()).is_ok());
    }
}
