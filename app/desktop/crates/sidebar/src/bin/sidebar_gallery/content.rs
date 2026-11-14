//! The stand-in for the panes area.

use gpui::{Context, IntoElement, ParentElement, Render, Styled, Window, div};
use ui::{Surface, Theme};

pub(crate) struct Content;

impl Render for Content {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<'_, Self>) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        div().size_full().bg(palette.background()).child("")
    }
}
