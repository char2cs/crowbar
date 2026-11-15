//! A flat view standing in for the panes area.

use gpui::{Context, Hsla, IntoElement, Render, Styled, Window, div};

#[derive(Default)]
pub(crate) struct Blank {
    pub(crate) background: Hsla,
}

impl Render for Blank {
    fn render(
        &mut self,
        _window: &mut Window,
        _cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        div().size_full().bg(self.background)
    }
}
