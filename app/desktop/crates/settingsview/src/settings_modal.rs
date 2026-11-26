//! The settings modal: a GPUI view over [`settings::State`] and the two backends.

mod close_handler;
mod field_input;
mod modal_config;
mod native_prompt;
mod run_intent;

pub use modal_config::ModalConfig;

use std::rc::Rc;

use chord::Primary;
use gpui::{
    App, AppContext as _, AsyncApp, Context, Entity, FocusHandle, IntoElement, ParentElement,
    Render, ScrollHandle, WeakEntity, Window, div,
};
use settings::{Change, RecordBackend, Subscription};
use ui::{Dialog, IconName, NativeAlert, TextInput, TextInputEvent, TextInputStyle};

use crate::modal::body;
use crate::settings_modal::close_handler::CloseHandler;

use crate::frame::Fields;
use crate::frame::Frame;
use crate::frame::RowLayout;
use crate::intent::Intent;
use crate::intent::Runner as _;
use crate::intent::Sink;
use crate::lift::Lift;
use crate::modal::notice_view;
use crate::modal::prompt_view;
use crate::modal::{extent, headed, search_field};

const SEARCH_PLACEHOLDER: &str = "Search settings...";

/// The settings modal view. Render it as the root of a window or as an overlay; it draws its own scrim.
pub struct SettingsModal {
    pub(crate) state: settings::State,
    pub(crate) settings: Rc<dyn settings::Backend>,
    pub(crate) records: Rc<dyn RecordBackend>,
    pub(crate) fields: Fields,
    pub(crate) search: Entity<TextInput>,
    pub(crate) scroll: ScrollHandle,
    pub(crate) focus: FocusHandle,
    pub(crate) notice: Option<String>,
    pub(crate) lift: Option<Lift>,
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
        let state = settings::State::new(
            config.schema,
            snapshot,
            config.page.as_ref(),
            Primary::native(),
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
            lift: None,
            on_close: config.on_close,
            _subscriptions: vec![typing],
            _changes: changes,
        }
    }

    /// Moves keyboard focus into the modal's search field, its first control, as develop's dialog does.
    pub fn focus(
        &self,
        window: &mut Window,
        cx: &mut App,
    ) {
        self.search
            .update(cx, |search, cx| search.focus(window, cx));
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
        let missing: Vec<settings::Row> = self.state.page().map_or_else(Vec::new, |page| {
            page.rows()
                .filter(|row| self.fields.get(row.key()).is_none())
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

fn sink_to(owner: Entity<SettingsModal>) -> Sink {
    Sink::new(move |intent, window, cx| {
        owner.update(cx, |modal, cx| modal.run(intent, window, cx));
    })
}

fn follow_backend(
    settings: &dyn settings::Backend,
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
            lift: self.lift.as_ref(),
            rows: RowLayout::of(window),
        };
        let content = body(&frame, (window, cx));
        let dialog = headed(Dialog::new("settings"), &frame, (window, cx))
            .flush(extent(window))
            .header_actions(search_field(&self.search, window))
            .body(content)
            .on_close(move |window, cx| closer.deliver(Intent::Close, (window, cx)));
        div()
            .child(dialog)
            .children(
                self.state
                    .prompt()
                    .filter(|_| NativeAlert::installed(cx).is_none())
                    .map(|prompt| prompt_view(&sink, prompt)),
            )
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
    use settings::{Key, Read as _, Value};

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, row, run};

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
        assert_eq!(harness.backend.get(target.key()), None);
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
    fn typing_right_after_opening_goes_into_the_search(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        harness
            .modal
            .update_in(cx, |modal, window, cx| modal.focus(window, cx));
        cx.simulate_input("git");
        assert_eq!(
            inspect(&harness, cx, |modal| modal.state.query().to_owned()),
            "git"
        );
    }
}
