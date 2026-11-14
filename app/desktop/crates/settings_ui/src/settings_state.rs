//! The pure state machine of the settings modal.

use std::collections::{BTreeMap, BTreeSet};

use crate::action::Action;
use crate::button_spec::ButtonSpec;
use crate::choice::Choice;
use crate::choice_source::ChoiceSource;
use crate::chord::Chord;
use crate::chord_capture::ChordCapture;
use crate::chord_source::ChordSource;
use crate::confirm::Confirm;
use crate::control::Control;
use crate::document::Document;
use crate::effect::Effect;
use crate::issue::Issue;
use crate::key::Key;
use crate::keymap::Keymap;
use crate::navigation::Navigation;
use crate::nudge::Nudge;
use crate::page::Page;
use crate::page_id::PageId;
use crate::pressed::Pressed;
use crate::primary::Primary;
use crate::prompt::Prompt;
use crate::rail_move::RailMove;
use crate::refusal::Refusal;
use crate::row::Row;
use crate::schema::Schema;
use crate::scope::Scope;
use crate::shortcut::Shortcut;
use crate::shortcut_group::ShortcutGroup;
use crate::snapshot::Snapshot;
use crate::value::Value;
use crate::visibility::Visibility;

/// Everything the modal remembers: the schema, the values on screen, the selected page, the search,
/// the field being edited, the shortcut being captured and the confirmation being asked.
///
/// Methods that change a setting return an [`Effect`]; the view performs it against the backend and
/// reports the outcome with [`SettingsState::acknowledged`] or [`SettingsState::write_failed`].
#[derive(Debug, Clone)]
pub struct SettingsState {
    schema: Schema,
    document: Document,
    capabilities: BTreeSet<String>,
    choices: BTreeMap<String, Vec<Choice>>,
    navigation: Navigation,
    query: String,
    visibility: Visibility,
    capture: ChordCapture,
    prompt: Option<Prompt>,
    primary: Primary,
}

impl SettingsState {
    /// A state over `schema` and `snapshot`, opened on `page` (the first visible page when `None` or unknown).
    pub fn new(
        schema: Schema,
        snapshot: Snapshot,
        page: Option<&PageId>,
        primary: Primary,
    ) -> Self {
        let document = Document::new(snapshot.values, schema.defaults().clone());
        let mut state = Self {
            document,
            capabilities: snapshot.capabilities,
            choices: snapshot.choices,
            navigation: Navigation::new(page.cloned().unwrap_or_else(|| PageId::new(""))),
            query: String::new(),
            visibility: Visibility::default(),
            capture: ChordCapture::default(),
            prompt: None,
            primary,
            schema,
        };
        state.refresh();
        state
    }

    fn scope(&self) -> Scope<'_> {
        Scope {
            values: self.document.values(),
            defaults: self.document.defaults(),
            capabilities: &self.capabilities,
        }
    }

    fn refresh(&mut self) {
        self.visibility = Visibility::compute(&self.schema, &self.scope(), &self.query);
        let visible: Vec<&PageId> = self.visibility.pages().iter().collect();
        self.navigation.reconcile(&visible);
    }

    /// The schema being shown.
    pub fn schema(&self) -> &Schema {
        &self.schema
    }

    /// The pages the rail lists: those whose condition holds and that the search leaves.
    pub fn visible_pages(&self) -> Vec<&Page> {
        let ids = self.visibility.pages();
        self.schema
            .pages()
            .iter()
            .filter(|page| ids.contains(&page.id))
            .collect()
    }

    /// The selected page, if any page is visible.
    pub fn page(&self) -> Option<&Page> {
        self.schema
            .page(self.navigation.active())
            .filter(|page| self.visibility.pages().contains(&page.id))
    }

    /// Selects `id` when it is visible.
    pub fn select_page(&mut self, id: &PageId) {
        let visible: Vec<&PageId> = self.visibility.pages().iter().collect();
        self.navigation.select(id, &visible);
    }

    /// Moves the rail selection with the keyboard.
    pub fn move_selection(&mut self, movement: RailMove) {
        let visible: Vec<&PageId> = self.visibility.pages().iter().collect();
        self.navigation.step(movement, &visible);
    }

    /// The search text.
    pub fn query(&self) -> &str {
        &self.query
    }

    /// Searches pages, sections, rows, descriptions, keywords and shortcuts; the selection moves to the first visible page when its own page drops out.
    pub fn set_query(&mut self, query: &str) {
        query.clone_into(&mut self.query);
        self.refresh();
    }

    /// Whether the search leaves `row` visible.
    pub fn row_visible(&self, row: &Row) -> bool {
        self.visibility.row(row)
    }

    /// Whether the search leaves `shortcut` visible.
    pub fn shortcut_visible(&self, shortcut: &Shortcut) -> bool {
        self.visibility.shortcut(shortcut)
    }

    /// Whether the search leaves the non-row block `index` of the selected page visible.
    pub fn block_visible(&self, index: usize) -> bool {
        self.visibility.block(self.navigation.active(), index)
    }

    /// Whether the switch of `row` shows as on.
    pub fn switch_on(&self, row: &Row) -> bool {
        let stored = self
            .document
            .value(&row.key)
            .and_then(Value::as_bool)
            .unwrap_or(false);
        let inverted = matches!(row.control, Control::Switch { invert: true });
        stored != inverted
    }

    /// The options of the select `row`.
    pub fn choices(&self, row: &Row) -> Vec<Choice> {
        let Control::Select { source, .. } = &row.control else {
            return Vec::new();
        };
        match source {
            ChoiceSource::Fixed(choices) => choices.clone(),
            ChoiceSource::Provided(name) => self.choices.get(name).cloned().unwrap_or_default(),
        }
    }

    /// The index of the option the select `row` currently stores.
    pub fn selected_index(&self, row: &Row) -> Option<usize> {
        let stored = self.document.value(&row.key)?.as_text()?;
        self.choices(row)
            .iter()
            .position(|choice| choice.value == stored)
    }

    /// The text of the field `row`: the draft being typed, or the stored value.
    pub fn field_text(&self, row: &Row) -> String {
        self.document.text(row)
    }

    /// The problem with what is typed in `row`, if any.
    pub fn issue(&self, row: &Row) -> Option<Issue> {
        let text = self.document.text(row);
        row.control.issue(&text)
    }

    /// Whether the field of `row` holds typing that is not stored yet.
    pub fn is_dirty(&self, row: &Row) -> bool {
        self.document.is_dirty(row)
    }

    /// Whether a write is still waiting for the backend.
    pub fn has_unsaved(&self) -> bool {
        self.document.has_unsaved()
    }

    /// Whether `row` shows its reset icon: it has a default and holds something else.
    pub fn resettable(&self, row: &Row) -> bool {
        row.default.is_some() && !self.document.is_default(row)
    }

    /// The description under the label: the invalid-value text while the field is invalid.
    pub fn description<'a>(&self, row: &'a Row) -> Option<&'a str> {
        if self.issue(row).is_some() {
            return row
                .invalid_description
                .as_deref()
                .or(row.description.as_deref());
        }
        if let Control::Button(spec) = &row.control
            && !self.button_enabled(spec)
        {
            return spec.disabled_note.as_deref().or(row.description.as_deref());
        }
        row.description.as_deref()
    }

    /// Flips the switch of `row`.
    pub fn toggle(&mut self, row: &Row) -> Option<Effect> {
        let stored = self.document.value(&row.key).and_then(Value::as_bool)?;
        Some(self.write(&row.key, Value::Bool(!stored)))
    }

    /// Stores the option at `index` of the select `row`.
    pub fn choose(&mut self, row: &Row, index: usize) -> Option<Effect> {
        let choice = self.choices(row).into_iter().nth(index)?;
        Some(self.write(&row.key, Value::Text(choice.value)))
    }

    /// Records typing in the field of `row` without storing it.
    pub fn edit(&mut self, row: &Row, text: &str) {
        self.document.edit(&row.key, text);
    }

    /// Stores what is typed in `row` (Enter, or the field losing focus); text that does not parse is dropped; text the validator refuses is stored and flagged.
    pub fn commit(&mut self, row: &Row) -> Option<Effect> {
        let text = self.document.draft(&row.key)?.to_owned();
        let parsed = row.control.parse(&text);
        let Ok(value) = parsed else {
            self.document.revert(&row.key);
            return None;
        };
        if self.document.value(&row.key) == Some(&value) {
            self.document.revert(&row.key);
            return None;
        }
        Some(self.write(&row.key, value))
    }

    /// Drops what is typed in `row` and shows the stored value again.
    pub fn revert(&mut self, row: &Row) {
        self.document.revert(&row.key);
    }

    /// Moves the number of `row` one step.
    pub fn nudge(&mut self, row: &Row, nudge: Nudge) -> Option<Effect> {
        let Control::Number { range, .. } = &row.control else {
            return None;
        };
        let typed = range.parse(&self.document.text(row));
        let current = typed.or_else(|| self.document.value(&row.key).and_then(Value::as_number))?;
        let next = range.stepped(current, nudge);
        Some(self.write(&row.key, Value::Number(next)))
    }

    /// Restores the default of `row`.
    pub fn reset(&mut self, row: &Row) -> Option<Effect> {
        let default = row.default.clone()?;
        if self.document.is_default(row) {
            self.document.revert(&row.key);
            return None;
        }
        Some(self.write(&row.key, default))
    }

    /// Whether the button `spec` can be pressed.
    pub fn button_enabled(&self, spec: &ButtonSpec) -> bool {
        spec.enabled
            .as_ref()
            .is_none_or(|rule| rule.holds(&self.scope()))
    }

    /// Presses `spec`: asks its confirmation first, or returns the effect at once.
    pub fn press_button(&mut self, spec: &ButtonSpec) -> Option<Effect> {
        if !self.button_enabled(spec) {
            return None;
        }
        let effect = match &spec.action {
            Action::Backend(id) => Effect::Invoke(id.clone()),
            Action::ResetOverrides(key) => self.clear_overrides(key)?,
        };
        self.confirm_or_run(spec.confirm.as_ref(), effect)
    }

    /// Runs `effect` after asking `confirm`, or at once when there is nothing to ask.
    pub fn confirm_or_run(&mut self, confirm: Option<&Confirm>, effect: Effect) -> Option<Effect> {
        let Some(confirm) = confirm else {
            return Some(effect);
        };
        self.prompt = Some(Prompt {
            confirm: confirm.clone(),
            then: effect,
        });
        None
    }

    /// The question waiting for an answer.
    pub fn prompt(&self) -> Option<&Prompt> {
        self.prompt.as_ref()
    }

    /// The user confirmed: the effect the question guarded.
    pub fn accept_prompt(&mut self) -> Option<Effect> {
        self.prompt.take().map(|prompt| prompt.then)
    }

    /// The user declined.
    pub fn dismiss_prompt(&mut self) {
        self.prompt = None;
    }

    /// The chord of `shortcut` and whether the user set it.
    pub fn chord(
        &self,
        group: &ShortcutGroup,
        shortcut: &Shortcut,
    ) -> (Option<Chord>, ChordSource) {
        let keymap = Keymap::new(&self.schema, &self.document);
        (
            keymap.chord(group, shortcut),
            keymap.source(group, shortcut),
        )
    }

    /// Labels of the other commands bound to the same chord as `shortcut`.
    pub fn conflicts(&self, group: &ShortcutGroup, shortcut: &Shortcut) -> Vec<String> {
        Keymap::new(&self.schema, &self.document).conflicts(group, shortcut)
    }

    /// Whether any shortcut has a custom chord.
    pub fn has_custom_chords(&self) -> bool {
        Keymap::new(&self.schema, &self.document).has_custom()
    }

    /// The label of `chord` on this platform.
    pub fn label(&self, chord: &Chord) -> String {
        chord.label(self.primary)
    }

    /// Starts (or, on the capturing button, stops) listening for a chord for `command`.
    pub fn toggle_capture(&mut self, command: &str) {
        self.capture.begin(command);
    }

    /// The command whose chord is being captured.
    pub fn capturing(&self) -> Option<&str> {
        self.capture.target()
    }

    /// Why the last key press was turned down.
    pub fn capture_refusal(&self) -> Option<Refusal> {
        self.capture.refusal()
    }

    /// Feeds a key press to the capture; returns the write when it completed a chord.
    pub fn press_key(&mut self, pressed: &Pressed) -> Option<Effect> {
        let (command, chord) = self.capture.feed(pressed, self.primary)?;
        self.set_chord(&command, Some(chord.as_str()))
    }

    /// Drops the custom chord of `command`, which restores its default.
    pub fn clear_chord(&mut self, command: &str) -> Option<Effect> {
        self.set_chord(command, None)
    }

    fn set_chord(&mut self, command: &str, chord: Option<&str>) -> Option<Effect> {
        let (group, shortcut) = self
            .schema
            .shortcuts()
            .find(|(_, s)| s.command == command)?;
        let mut map = Keymap::new(&self.schema, &self.document).overrides(group);
        let unchanged_default =
            chord.is_some_and(|text| shortcut.default.as_ref().map(Chord::as_str) == Some(text));
        match chord {
            Some(text) if !unchanged_default => map.insert(command.to_owned(), text.to_owned()),
            _ => map.remove(command),
        };
        let key = group.overrides.clone();
        Some(self.write(&key, Value::Map(map)))
    }

    fn clear_overrides(&mut self, key: &Key) -> Option<Effect> {
        let has_any = self
            .document
            .value(key)
            .and_then(Value::as_map)
            .is_some_and(|map| !map.is_empty());
        has_any.then(|| self.write(key, Value::Map(BTreeMap::new())))
    }

    fn write(&mut self, key: &Key, value: Value) -> Effect {
        let effect = self.document.apply(key, value);
        self.refresh();
        effect
    }

    /// The backend stored `key`.
    pub fn acknowledged(&mut self, key: &Key) {
        self.document.acknowledge(key);
    }

    /// The backend refused `key`: its value goes back; returns the label of the row for the error toast.
    pub fn write_failed(&mut self, key: &Key) -> Option<String> {
        self.document.fail(key);
        self.refresh();
        let row_label = self
            .schema
            .rows()
            .find(|row| &row.key == key)
            .map(|row| row.label.clone());
        row_label.or_else(|| Some(key.to_string()))
    }

    /// The backend reports a new value for `key`.
    pub fn remote_changed(&mut self, key: &Key, value: Value) {
        self.document.remote(key, value);
        self.refresh();
    }
}

#[cfg(test)]
#[path = "settings_state_tests.rs"]
mod tests;
