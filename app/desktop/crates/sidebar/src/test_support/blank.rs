//! A flat view standing in for the panes area.

use gpui::{Context, IntoElement, Render, Styled, Window, div};

pub(crate) struct Blank;

impl Render for Blank {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<'_, Self>) -> impl IntoElement {
        div().size_full()
    }
}
