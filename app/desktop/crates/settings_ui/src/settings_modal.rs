//! The settings modal: a GPUI view over [`SettingsState`] and the two backends.

use std::rc::Rc;

use gpui::{
    AppContext as _, AsyncApp, Context, Entity, FocusHandle, IntoElement, ParentElement, Render,
    ScrollHandle, WeakEntity, Window, div,
};
use ui::{Dialog, IconName, TextInput, TextInputEvent, TextInputStyle};

use crate::body::body;
use crate::change::Change;
use crate::close_handler::CloseHandler;
use crate::field_input;
use crate::fields::Fields;
use crate::frame::Frame;
use crate::header::{extent, search_field};
use crate::intent::Intent;
use crate::intent_runner::IntentRunner as _;
use crate::intent_sink::IntentSink;
use crate::modal_config::ModalConfig;
use crate::notice_view::notice_view;
use crate::prompt_view::prompt_view;
use crate::record_backend::RecordBackend;
use crate::settings_backend::SettingsBackend;
use crate::settings_state::SettingsState;
use crate::subscription::Subscription;

const SEARCH_PLACEHOLDER: &str = "Search settings...";
const TITLE: &str = "Settings";

/// The settings modal view. Render it as the root of a window or as an overlay; it draws its own scrim.
pub struct SettingsModal {
    pub(crate) state: SettingsState,
    pub(crate) settings: Rc<dyn SettingsBackend>,
    pub(crate) records: Rc<dyn RecordBackend>,
    pub(crate) fields: Fields,
    pub(crate) search: Entity<TextInput>,
    pub(crate) scroll: ScrollHandle,
    pub(crate) focus: FocusHandle,
    pub(crate) notice: Option<String>,
    pub(crate) on_close: Option<CloseHandler>,
    _subscriptions: Vec<gpui::Subscription>,
    _changes: Subscription,
}

impl SettingsModal {
    /// Builds the modal over `config`, reading the backend snapshot once.
    pub fn new(
        config: ModalConfig,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let snapshot = config.settings.snapshot();
        let state = SettingsState::new(
            config.schema,
            snapshot,
            config.page.as_ref(),
            config.primary,
        );
        let search = cx.new(|cx| {
            TextInput::new(SEARCH_PLACEHOLDER, cx)
                .style(TextInputStyle::Compact)
                .leading_icon(IconName::MagnifyingGlass)
        });
        let typing = cx.subscribe_in(
            &search,
            window,
            |modal, _, event: &TextInputEvent, window, cx| {
                modal.run(Intent::Search(event.clone()), window, cx);
            },
        );
        let changes = follow_backend(config.settings.as_ref(), cx);
        Self {
            state,
            settings: config.settings,
            records: config.records,
            fields: Fields::default(),
            search,
            scroll: ScrollHandle::new(),
            focus: cx.focus_handle(),
            notice: None,
            on_close: config.on_close,
            _subscriptions: vec![typing],
            _changes: changes,
        }
    }

    /// Moves keyboard focus into the modal so that Escape and the arrow keys reach it.
    pub fn focus(
        &self,
        window: &mut Window,
    ) {
        self.focus.focus(window);
    }

    /// Starts with `query` typed into the search field.
    #[must_use]
    pub fn with_query(
        mut self,
        query: Option<&str>,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let Some(query) = query else {
            return self;
        };
        self.state.set_query(query);
        self.search
            .update(cx, |input, cx| input.set_text(query, cx));
        self
    }

    fn on_change(
        &mut self,
        change: &Change,
        cx: &mut Context<'_, Self>,
    ) {
        if let Change::Setting(key, value) = change {
            self.state.remote_changed(key, value.clone());
            self.fields.sync(&self.state, cx);
        }
        cx.notify();
    }

    fn ensure_fields(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let missing: Vec<crate::row::Row> = self.state.page().map_or_else(Vec::new, |page| {
            page.rows()
                .filter(|row| self.fields.get(&row.key).is_none())
                .cloned()
                .collect()
        });
        for row in &missing {
            field_input::build(
                &mut self.fields,
                (row, &self.state.field_text(row)),
                (window, cx),
            );
        }
    }
}

fn sink_to(owner: Entity<SettingsModal>) -> IntentSink {
    IntentSink::new(move |intent, window, cx| {
        owner.update(cx, |modal, cx| modal.run(intent, window, cx));
    })
}

fn follow_backend(
    settings: &dyn SettingsBackend,
    cx: &mut Context<'_, SettingsModal>,
) -> Subscription {
    let weak = cx.weak_entity();
    let handle = cx.to_async();
    settings.subscribe(Rc::new(move |change| {
        let (weak, change) = (weak.clone(), change.clone());
        handle
            .spawn(async move |cx| forward(&weak, &change, cx))
            .detach();
    }))
}

fn forward(
    weak: &WeakEntity<SettingsModal>,
    change: &Change,
    cx: &mut AsyncApp,
) {
    drop(weak.update(cx, |modal, cx| modal.on_change(change, cx)));
}

impl Render for SettingsModal {
    fn render(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        self.ensure_fields(window, cx);
        let sink = sink_to(cx.entity());
        let closer = sink.clone();
        let frame = Frame {
            state: &self.state,
            sink: &sink,
            fields: &self.fields,
            records: self.records.as_ref(),
            focus: &self.focus,
            scroll: &self.scroll,
        };
        let content = body(&frame, (window, cx));
        let dialog = Dialog::new("settings")
            .title(TITLE)
            .flush(extent(window))
            .header_actions(search_field(&self.search, window))
            .body(content)
            .on_close(move |window, cx| closer.deliver(Intent::Close, (window, cx)));
        div()
            .child(dialog)
            .children(self.state.prompt().map(|prompt| prompt_view(&sink, prompt)))
            .children(self.notice.as_deref().map(notice_view))
    }
}

impl std::fmt::Debug for SettingsModal {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("SettingsModal")
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::intent::Intent;
    use crate::key::Key;
    use crate::settings_read::SettingsRead as _;
    use crate::test_support::{inspect, open, row, run};
    use crate::value::Value;

    #[gpui::test]
    fn a_toggle_reaches_the_backend_and_the_row_becomes_resettable(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        let target = row("sidebarFollowsFocus");
        run(&harness, Intent::Toggle(target.clone()), cx);
        assert_eq!(
            harness.backend.get(&Key::new("sidebarFollowsFocus")),
            Some(Value::Bool(true))
        );
        assert!(inspect(&harness, cx, |modal| modal
            .state
            .resettable(&target)));
    }

    #[gpui::test]
    fn a_refused_write_goes_back_and_says_so(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        let target = row("sidebarFollowsFocus");
        harness.backend.refuse_writes();
        run(&harness, Intent::Toggle(target.clone()), cx);
        assert_eq!(harness.backend.get(&target.key), None);
        assert!(!inspect(&harness, cx, |modal| modal
            .state
            .switch_on(&target)));
        assert!(inspect(&harness, cx, |modal| modal.notice.is_some()));
        run(&harness, Intent::Select("editor".into()), cx);
        assert!(inspect(&harness, cx, |modal| modal.notice.is_none()));
    }

    #[gpui::test]
    fn a_change_made_elsewhere_shows_up(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        harness
            .backend
            .publish(&Key::new("sidebarFollowsFocus"), Value::Bool(true));
        cx.run_until_parked();
        assert!(inspect(&harness, cx, |modal| modal
            .state
            .switch_on(&row("sidebarFollowsFocus"))));
    }

    #[gpui::test]
    fn closing_calls_the_close_handler_once(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        run(&harness, Intent::Close, cx);
        assert_eq!(harness.closed.get(), 1);
    }

    #[gpui::test]
    fn every_page_renders_with_and_without_a_search(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        let pages: Vec<_> = inspect(&harness, cx, |modal| {
            modal
                .state
                .visible_pages()
                .iter()
                .map(|page| page.id.clone())
                .collect()
        });
        assert_eq!(pages.len(), 10);
        for page in pages {
            run(&harness, Intent::Select(page.clone()), cx);
            assert_eq!(
                inspect(&harness, cx, |modal| modal
                    .state
                    .page()
                    .map(|page| page.id.clone())),
                Some(page)
            );
        }
        let typed = Intent::Search(ui::TextInputEvent::Changed("zzzzzz".to_owned()));
        run(&harness, typed, cx);
        assert!(inspect(&harness, cx, |modal| modal
            .state
            .visible_pages()
            .is_empty()));
    }

    #[gpui::test]
    fn the_query_option_fills_the_search_field(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let backend = crate::test_support::backend();
        let config = crate::modal_config::ModalConfig::new(
            crate::catalog::default_schema().unwrap(),
            std::rc::Rc::new(backend.clone()),
            std::rc::Rc::new(backend),
        );
        let (modal, cx) = cx.add_window_view(|window, cx| {
            crate::settings_modal::SettingsModal::new(config, window, cx)
                .with_query(Some("font"), cx)
        });
        cx.run_until_parked();
        assert_eq!(
            modal.read_with(cx, |modal, _| modal.state.query().to_owned()),
            "font"
        );
    }
}
