//! The find bar over the terminal's top-right corner.

use gpui::{
    AppContext, Context, Div, Entity, EventEmitter, FontWeight, IntoElement, ParentElement, Render,
    SharedString, Styled, Subscription, Window, div, px,
};
use terminal::{SearchOptions, SearchQuery, SearchSummary};
use ui::{
    Button, ButtonSize, ButtonVariant, IconName, Surface, TextInput, TextInputEvent,
    TextInputStyle, Theme,
};

use crate::search_bar_event::SearchBarEvent;

const BAR_INSET: f32 = 8.0;
const INPUT_WIDTH: f32 = 220.0;
const BAR_GAP: f32 = 4.0;
const BAR_PADDING: f32 = 6.0;
const COUNT_WIDTH: f32 = 44.0;
const PLACEHOLDER: &str = "Find in terminal...";

pub(crate) struct SearchBar {
    input: Entity<TextInput>,
    options: SearchOptions,
    summary: SearchSummary,
    _input_events: Subscription,
}

impl EventEmitter<SearchBarEvent> for SearchBar {}

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
        input.read(cx).focus(window);
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
        cx: &gpui::App,
    ) {
        self.input.read(cx).focus(window);
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
            TextInputEvent::Submitted => cx.emit(SearchBarEvent::Next),
            TextInputEvent::Cancelled => cx.emit(SearchBarEvent::Close),
            _ => {},
        }
    }

    fn emit_query(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let query = self.query(cx);
        cx.emit(SearchBarEvent::QueryChanged(query));
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

    fn counter(
        &self,
        cx: &gpui::App,
    ) -> SharedString {
        if self.input.read(cx).text().is_empty() {
            return SharedString::default();
        }
        format!("{}/{}", self.summary.current, self.summary.total).into()
    }

    fn toggle_button(
        &self,
        id: &'static str,
        label: &'static str,
        option: SearchOptions,
        cx: &mut Context<'_, Self>,
    ) -> Button {
        let active = self.options.contains(option);
        let variant = match active {
            true => ButtonVariant::Secondary,
            false => ButtonVariant::Ghost,
        };
        let me = cx.entity();
        Button::new(id, label)
            .variant(variant)
            .size(ButtonSize::Xs)
            .on_click(move |_, _, app| me.update(app, |bar, cx| bar.toggle(option, cx)))
    }

    fn event_button(
        id: &'static str,
        icon: IconName,
        event: SearchBarEvent,
        cx: &mut Context<'_, Self>,
    ) -> Button {
        let me = cx.entity();
        Button::icon_only(id, icon)
            .variant(ButtonVariant::Ghost)
            .size(ButtonSize::Xs)
            .on_click(move |_, _, app| me.update(app, |_, cx| cx.emit(event.clone())))
    }
}

impl SearchBar {
    fn toolbar(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        div()
            .child(self.toggle_button("search-case", "Aa", SearchOptions::CASE_SENSITIVE, cx))
            .child(self.toggle_button("search-word", "W", SearchOptions::WHOLE_WORD, cx))
            .child(self.toggle_button("search-regex", ".*", SearchOptions::REGEX, cx))
            .child(Self::event_button(
                "search-previous",
                IconName::CaretUp,
                SearchBarEvent::Previous,
                cx,
            ))
            .child(Self::event_button(
                "search-next",
                IconName::CaretDown,
                SearchBarEvent::Next,
                cx,
            ))
            .child(Self::event_button(
                "search-close",
                IconName::X,
                SearchBarEvent::Close,
                cx,
            ))
            .flex()
            .items_center()
            .gap(px(BAR_GAP))
    }
}

impl Render for SearchBar {
    fn render(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let tokens = Theme::of(cx).palette(Surface::Window);
        let counter = self.counter(cx);
        let toolbar = self.toolbar(cx);
        div()
            .absolute()
            .top(px(BAR_INSET))
            .right(px(BAR_INSET))
            .flex()
            .items_center()
            .gap(px(BAR_GAP))
            .p(px(BAR_PADDING))
            .rounded(ui::radius::LG)
            .border_1()
            .border_color(tokens.border())
            .bg(tokens.popover())
            .child(div().w(px(INPUT_WIDTH)).child(self.input.clone()))
            .child(
                div()
                    .w(px(COUNT_WIDTH))
                    .text_size(ui::text_size::XS)
                    .font_weight(FontWeight::MEDIUM)
                    .text_color(tokens.muted_foreground())
                    .child(counter),
            )
            .child(toolbar)
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
        let queries = record_events::<SearchBar, SearchBarEvent>(cx, &bar);
        bar.update(cx, |bar, cx| bar.toggle(SearchOptions::REGEX, cx));
        let emitted = queries.borrow().first().cloned();
        let expected = SearchQuery {
            text: String::new(),
            options: SearchOptions::REGEX,
        };
        assert_eq!(emitted, Some(SearchBarEvent::QueryChanged(expected)));
        let summary = SearchSummary {
            current: 1,
            total: 2,
        };
        bar.update(cx, |bar, cx| bar.show_summary(summary, cx));
        assert!(bar.read_with(cx, SearchBar::counter).is_empty());
    }
}
