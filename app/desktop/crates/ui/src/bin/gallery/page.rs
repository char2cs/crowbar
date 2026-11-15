//! The gallery window.

use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, Context, Entity, InteractiveElement, IntoElement, ParentElement, Render,
    ScrollHandle, StatefulInteractiveElement, Styled, Window, div,
};
use ui::{Surface, TextInput, Theme, spacing};

use crate::section::section;
use crate::{buttons, feedback, icons, inputs, overlays, structure, toggles, typography};

pub(crate) struct Page {
    only: Option<String>,
    pub(crate) switch_on: bool,
    pub(crate) checked: bool,
    pub(crate) tab: usize,
    pub(crate) segment: usize,
    pub(crate) choice: Option<usize>,
    pub(crate) input: Entity<TextInput>,
    pub(crate) scroll: ScrollHandle,
}

impl Page {
    pub(crate) fn new(
        only: Option<String>,
        cx: &mut Context<'_, Self>,
    ) -> Self {
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

    pub(crate) fn edit(
        &mut self,
        cx: &mut Context<'_, Self>,
        change: impl FnOnce(&mut Self),
    ) {
        change(self);
        cx.notify();
    }

    fn wanted(
        &self,
        name: &str,
    ) -> bool {
        self.only.as_deref().is_none_or(|only| only == name)
    }

    fn sections(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Vec<(&'static str, &'static str, AnyElement)> {
        let page = cx.entity();
        vec![
            ("buttons", "BUTTONS", buttons::buttons().into_any_element()),
            (
                "inputs",
                "INPUTS",
                inputs::inputs(self, &page).into_any_element(),
            ),
            (
                "toggles",
                "TOGGLES AND TABS",
                toggles::toggles(self, &page).into_any_element(),
            ),
            (
                "feedback",
                "FEEDBACK",
                feedback::feedback().into_any_element(),
            ),
            (
                "structure",
                "STRUCTURE",
                structure::structure(&self.scroll).into_any_element(),
            ),
            ("icons", "ICONS", icons::icons().into_any_element()),
            ("overlays", "OVERLAYS", overlays::menus().into_any_element()),
            (
                "typography",
                "TYPOGRAPHY",
                typography::typography().into_any_element(),
            ),
        ]
    }
}

impl Render for Page {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let shown: Vec<AnyElement> = self
            .sections(cx)
            .into_iter()
            .filter(|(name, ..)| self.wanted(name))
            .map(|(_, title, body)| section(title, body, &theme).into_any_element())
            .collect();
        div()
            .id("gallery")
            .size_full()
            .overflow_y_scroll()
            .bg(palette.background())
            .text_color(palette.foreground())
            .p(spacing::STEP_4)
            .flex()
            .flex_col()
            .gap(spacing::STEP_5)
            .children(shown)
            .when(self.only.as_deref() == Some("dialog"), |page| {
                page.child(overlays::dialog())
            })
            .when(self.only.as_deref() == Some("plain-dialog"), |page| {
                page.child(overlays::plain_dialog())
            })
    }
}
