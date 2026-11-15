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
