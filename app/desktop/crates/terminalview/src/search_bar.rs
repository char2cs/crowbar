//! The find bar over the terminal's top-right corner.

mod card;
mod event;
mod session;
mod toggle_spec;

pub(crate) use event::Event;
pub(crate) use session::Session;

use gpui::{
    AppContext, Context, Entity, EventEmitter, InteractiveElement, IntoElement, ParentElement,
    Render, Styled, Subscription, Window, div, px,
};
use terminal::{SearchOptions, SearchQuery, SearchSummary};
use ui::{IconName, Surface, TextInput, TextInputEvent, TextInputStyle, Theme};

const BAR_INSET: f32 = 8.0;
const CARD_WIDTH: f32 = 320.0;
const CARD_PADDING: f32 = 6.0;
const CARD_OPACITY: f32 = 0.95;
const PLACEHOLDER: &str = "Find in terminal...";

pub(crate) struct SearchBar {
    input: Entity<TextInput>,
    options: SearchOptions,
    summary: SearchSummary,
    _input_events: Subscription,
}

impl EventEmitter<Event> for SearchBar {}

impl SearchBar {
    pub(crate) fn new(
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let input = cx.new(|cx| {
            TextInput::new(PLACEHOLDER, cx)
                .on(Surface::Window)
                .style(TextInputStyle::Compact)
                .leading_icon(IconName::MagnifyingGlass)
        });
        input.update(cx, |field, cx| field.focus(window, cx));
        let subscription = cx.subscribe(&input, Self::on_input);
        Self {
            input,
            options: SearchOptions::NONE,
            summary: SearchSummary::default(),
            _input_events: subscription,
        }
    }

    pub(crate) fn focus(
        &self,
        window: &mut Window,
        cx: &mut gpui::App,
    ) {
        self.input.update(cx, |field, cx| field.focus(window, cx));
    }

    pub(crate) fn show_summary(
        &mut self,
        summary: SearchSummary,
        cx: &mut Context<'_, Self>,
    ) {
        self.summary = summary;
        cx.notify();
    }

    pub(crate) fn query(
        &self,
        cx: &gpui::App,
    ) -> SearchQuery {
        SearchQuery {
            text: self.input.read(cx).text().to_owned(),
            options: self.options,
        }
    }

    fn on_input(
        &mut self,
        _: Entity<TextInput>,
        event: &TextInputEvent,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            TextInputEvent::Changed(_) => self.emit_query(cx),
            TextInputEvent::Submitted => cx.emit(Event::Next),
            TextInputEvent::Cancelled => cx.emit(Event::Close),
            _ => {},
        }
    }

    fn emit_query(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let query = self.query(cx);
        cx.emit(Event::QueryChanged(query));
    }

    fn toggle(
        &mut self,
        option: SearchOptions,
        cx: &mut Context<'_, Self>,
    ) {
        self.options = self.options.toggled(option);
        cx.notify();
        self.emit_query(cx);
    }
}

impl Render for SearchBar {
    fn render(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let tokens = Theme::of(cx).palette(Surface::Window);
        div()
            .absolute()
            .top(px(BAR_INSET))
            .right(px(BAR_INSET))
            .w(px(CARD_WIDTH))
            .p(px(CARD_PADDING))
            .rounded(ui::radius::XL)
            .border_1()
            .border_color(tokens.border())
            .bg(tokens.background().opacity(CARD_OPACITY))
            .capture_key_down(cx.listener(|_, event, _, cx| Self::on_card_key(event, cx)))
            .child(self.search_row(cx))
            .child(self.option_row(cx))
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::{open, record_events};

    #[gpui::test]
    fn toggling_an_option_emits_the_new_query(cx: &mut TestAppContext) {
        let (_, cx) = open(cx);
        let (bar, cx) = cx.add_window_view(SearchBar::new);
        let queries = record_events::<SearchBar, Event>(cx, &bar);
        bar.update(cx, |bar, cx| bar.toggle(SearchOptions::REGEX, cx));
        let emitted = queries.borrow().first().cloned();
        let expected = SearchQuery {
            text: String::new(),
            options: SearchOptions::REGEX,
        };
        assert_eq!(emitted, Some(Event::QueryChanged(expected)));
        let summary = SearchSummary {
            current: 1,
            total: 2,
        };
        bar.update(cx, |bar, cx| bar.show_summary(summary, cx));
        assert_eq!(
            bar.read_with(cx, SearchBar::counter),
            None,
            "no count while empty"
        );
    }
}
