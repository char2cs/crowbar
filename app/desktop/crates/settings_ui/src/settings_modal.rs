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
use crate::dispatch::deliver;
use crate::effect::Effect;
use crate::field_input;
use crate::fields::Fields;
use crate::frame::Frame;
use crate::header::{extent, search_field};
use crate::intent::Intent;
use crate::modal_config::ModalConfig;
use crate::notice_view::notice_view;
use crate::primary::Primary;
use crate::prompt_view::prompt_view;
use crate::record_backend::RecordBackend;
use crate::settings_backend::SettingsBackend;
use crate::settings_state::SettingsState;
use crate::subscription::Subscription;

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
    pub fn new(config: ModalConfig, window: &mut Window, cx: &mut Context<'_, Self>) -> Self {
        let snapshot = config.settings.snapshot();
        let state = SettingsState::new(
            config.schema,
            snapshot,
            config.page.as_ref(),
            Primary::native(),
        );
        let search = cx.new(|cx| {
            TextInput::new("Search settings...", cx)
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

    /// Starts with `query` typed into the search field.
    #[must_use]
    pub fn with_query(mut self, query: Option<&str>, cx: &mut Context<'_, Self>) -> Self {
        let Some(query) = query else {
            return self;
        };
        self.state.set_query(query);
        self.search
            .update(cx, |input, cx| input.set_text(query, cx));
        self
    }

    fn on_change(&mut self, change: &Change, cx: &mut Context<'_, Self>) {
        if let Change::Setting(key, value) = change {
            self.state.remote_changed(key, value.clone());
            self.fields.sync(&self.state, cx);
        }
        cx.notify();
    }

    pub(crate) fn perform(&mut self, effect: Option<Effect>) {
        let Some(effect) = effect else {
            return;
        };
        let failure = match effect {
            Effect::Write(key, value) => match self.settings.set(&key, value) {
                Ok(()) => {
                    self.state.acknowledged(&key);
                    None
                },
                Err(error) => {
                    let label = self.state.write_failed(&key).unwrap_or_default();
                    Some(format!("Could not save {label}. {error}"))
                },
            },
            Effect::Invoke(id) => self
                .settings
                .invoke(&id)
                .err()
                .map(|error| error.to_string()),
            Effect::Record {
                source,
                record,
                command,
            } => self
                .records
                .apply(&source, &record, command)
                .err()
                .map(|error| error.to_string()),
        };
        if failure.is_some() {
            self.notice = failure;
        }
    }

    pub(crate) fn ensure_fields(&mut self, window: &mut Window, cx: &mut Context<'_, Self>) {
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

fn forward(weak: &WeakEntity<SettingsModal>, change: &Change, cx: &mut AsyncApp) {
    drop(weak.update(cx, |modal, cx| modal.on_change(change, cx)));
}

impl Render for SettingsModal {
    fn render(&mut self, window: &mut Window, cx: &mut Context<'_, Self>) -> impl IntoElement {
        self.ensure_fields(window, cx);
        let owner = cx.entity();
        let closer = owner.clone();
        let frame = Frame {
            state: &self.state,
            owner: &owner,
            fields: &self.fields,
            records: self.records.as_ref(),
        };
        let content = body(&frame, (&self.focus, &self.scroll), (window, cx));
        let dialog = Dialog::new("settings")
            .title("Settings")
            .flush(extent(window))
            .header_actions(search_field(&self.search, window))
            .body(content)
            .on_close(move |window, cx| deliver(&closer, Intent::Close, (window, cx)));
        div()
            .child(dialog)
            .children(
                self.state
                    .prompt()
                    .map(|prompt| prompt_view(&owner, prompt)),
            )
            .children(self.notice.as_deref().map(notice_view))
    }
}

impl std::fmt::Debug for SettingsModal {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("SettingsModal")
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
#[path = "settings_modal_tests.rs"]
mod tests;
