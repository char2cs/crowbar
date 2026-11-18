//! The find bar of the view.

use gpui::{AppContext, Context, Window};
use terminal::{SearchQuery, SearchSummary};

use crate::search_bar::SearchBar;
use crate::search_bar::SearchBarEvent;
use crate::search_bar::SearchSession;
use crate::terminal_view::TerminalView;

impl TerminalView {
    pub(crate) fn open_search(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(session) = &self.search {
            session.bar.read(cx).focus(window, cx);
            return;
        }
        let bar = cx.new(|cx| SearchBar::new(window, cx));
        let events = cx.subscribe_in(&bar, window, |this, _bar, event, window, cx| {
            this.on_search_event(event, window, cx);
        });
        self.search = Some(SearchSession {
            bar,
            _events: events,
        });
        cx.notify();
    }

    pub(crate) fn close_search(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.search = None;
        self.terminal.clear_search();
        window.focus(&self.focus);
        cx.notify();
    }

    fn on_search_event(
        &mut self,
        event: &SearchBarEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            SearchBarEvent::QueryChanged(query) => self.search_for(query.clone(), cx),
            SearchBarEvent::Next => {
                let summary = self.terminal.search_next();
                self.show_summary(summary, cx);
            },
            SearchBarEvent::Previous => {
                let summary = self.terminal.search_previous();
                self.show_summary(summary, cx);
            },
            SearchBarEvent::Close => self.close_search(window, cx),
        }
    }

    fn search_for(
        &mut self,
        query: SearchQuery,
        cx: &mut Context<'_, Self>,
    ) {
        if query.text.is_empty() {
            self.terminal.clear_search();
            self.show_summary(SearchSummary::default(), cx);
            return;
        }
        let summary = self.terminal.search(query).unwrap_or_default();
        self.show_summary(summary, cx);
    }

    fn show_summary(
        &mut self,
        summary: SearchSummary,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(session) = &self.search {
            session
                .bar
                .update(cx, |bar, cx| bar.show_summary(summary, cx));
        }
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use terminal::{SearchOptions, SearchQuery};

    use crate::search_bar::SearchBarEvent;
    use crate::test_support::{draw, feed, focus, open};

    #[gpui::test]
    fn the_find_shortcut_opens_the_bar_and_escape_closes_it(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        feed(&harness, cx, "cat dog cat");
        focus(&harness, cx);
        cx.simulate_keystrokes("ctrl-f");
        draw(cx);
        assert!(harness.view.read_with(cx, |view, _| view.search.is_some()));
        let bar = harness.view.read_with(cx, |view, _| {
            view.search.as_ref().map(|session| session.bar.clone())
        });
        let query = SearchQuery {
            text: "cat".into(),
            options: SearchOptions::NONE,
        };
        if let Some(bar) = bar {
            bar.update(cx, |_, cx| cx.emit(SearchBarEvent::QueryChanged(query)));
        }
        let total = harness
            .view
            .read_with(cx, |view, _| view.terminal.snapshot().matches.len());
        assert_eq!(total, 1);
        cx.simulate_keystrokes("escape");
        assert!(harness.view.read_with(cx, |view, _| view.search.is_none()));
        assert!(
            harness
                .view
                .read_with(cx, |view, _| view.terminal.snapshot().matches.is_empty())
        );
    }
}
