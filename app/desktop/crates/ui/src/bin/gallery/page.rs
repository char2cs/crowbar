//! The gallery window.

use gpui::{
    Context, Entity, FontWeight, IntoElement, ParentElement, Render, ScrollHandle, SharedString, Styled, Window, div, px,
};
use ui::{Surface, TextInput, Theme};

/// State of the demo controls.
pub struct Page {
    pub(crate) only: Option<String>,
    pub(crate) switch_on: bool,
    pub(crate) checked: bool,
    pub(crate) tab: usize,
    pub(crate) segment: usize,
    pub(crate) choice: Option<usize>,
    pub(crate) input: Entity<TextInput>,
    pub(crate) scroll: ScrollHandle,
}

impl Page {
    /// A page showing everything, or only the `only` section.
    pub fn new(only: Option<String>, _window: &mut Window, cx: &mut Context<Self>) -> Self {
        Self {
            only,
            switch_on: true,
            checked: true,
            tab: 0,
            segment: 0,
            choice: Some(1),
            input: TextInput::build("Search", cx),
            scroll: ScrollHandle::new(),
        }
    }

    fn wanted(&self, name: &str) -> bool {
        self.only.as_deref().is_none_or(|only| only == name)
    }

    fn section(&self, title: &'static str, body: impl IntoElement, cx: &Context<Self>) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        div()
            .flex()
            .flex_col()
            .gap(px(8.0))
            .child(div().text_size(px(11.0)).font_weight(FontWeight::MEDIUM).text_color(palette.muted_foreground()).child(SharedString::from(title)))
            .child(body)
    }
}

impl Render for Page {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let mut page = div().id("gallery").size_full().overflow_y_scroll().bg(palette.background()).text_color(palette.foreground()).p(px(16.0)).flex().flex_col().gap(px(20.0));
        if self.wanted("buttons") {
            let body = self.buttons(cx);
            page = page.child(self.section("BUTTONS", body, cx));
        }
        if self.wanted("inputs") {
            let body = self.inputs(cx);
            page = page.child(self.section("INPUTS", body, cx));
        }
        if self.wanted("toggles") {
            let body = self.toggles(cx);
            page = page.child(self.section("TOGGLES AND TABS", body, cx));
        }
        if self.wanted("feedback") {
            let body = self.feedback(cx);
            page = page.child(self.section("FEEDBACK", body, cx));
        }
        if self.wanted("structure") {
            let body = self.structure(cx);
            page = page.child(self.section("STRUCTURE", body, cx));
        }
        if self.wanted("icons") {
            let body = self.icons(cx);
            page = page.child(self.section("ICONS", body, cx));
        }
        if self.wanted("overlays") || self.only.as_deref() == Some("dialog") {
            page = page.child(self.overlays(window, cx));
        }
        page
    }
}
