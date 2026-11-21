//! A flat view standing in for the panes area, counting how often it renders.

use gpui::{Context, Hsla, IntoElement, Render, Styled, Window, div};

#[derive(Default)]
pub(crate) struct Blank {
    pub(crate) background: Hsla,
    pub(crate) renders: usize,
}

impl Render for Blank {
    fn render(
        &mut self,
        _window: &mut Window,
        _cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        self.renders += 1;
        div().size_full().bg(self.background)
    }
}
