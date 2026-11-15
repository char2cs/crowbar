//! The pure state machine of the settings modal.

use std::collections::{BTreeMap, BTreeSet};

use crate::action::Action;
use crate::button_spec::ButtonSpec;
use crate::choice::Choice;
use crate::choice_source::ChoiceSource;
use crate::chord_capture::ChordCapture;
use crate::confirm::Confirm;
use crate::control::Control;
use crate::document::Document;
use crate::effect::Effect;
use crate::issue::Issue;
use crate::key::Key;
use crate::navigation::Navigation;
use crate::nudge::Nudge;
use crate::page::Page;
use crate::page_id::PageId;
use crate::primary::Primary;
use crate::prompt::Prompt;
use crate::rail_move::RailMove;
use crate::row::Row;
use crate::schema::Schema;
use crate::scope::Scope;
use crate::shortcut::Shortcut;
use crate::snapshot::Snapshot;
use crate::value::Value;
use crate::visibility::Visibility;

mod shortcuts;

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
    pub fn select_page(
        &mut self,
        id: &PageId,
    ) {
        let visible: Vec<&PageId> = self.visibility.pages().iter().collect();
        self.navigation.select(id, &visible);
    }

    /// Moves the rail selection with the keyboard.
    pub fn move_selection(
        &mut self,
        movement: RailMove,
    ) {
        let visible: Vec<&PageId> = self.visibility.pages().iter().collect();
        self.navigation.step(movement, &visible);
    }

    /// The search text.
    pub fn query(&self) -> &str {
        &self.query
    }

    /// Searches pages, sections, rows, descriptions, keywords and shortcuts; the selection moves to the first visible page when its own page drops out.
    pub fn set_query(
        &mut self,
        query: &str,
    ) {
        query.clone_into(&mut self.query);
        self.refresh();
    }

    /// Whether the search leaves `row` visible.
    pub fn row_visible(
        &self,
        row: &Row,
    ) -> bool {
        self.visibility.row(row)
    }

    /// Whether the search leaves `shortcut` visible.
    pub fn shortcut_visible(
        &self,
        shortcut: &Shortcut,
    ) -> bool {
        self.visibility.shortcut(shortcut)
    }

    /// Whether the search leaves the non-row block `index` of the selected page visible.
    pub fn block_visible(
        &self,
        index: usize,
    ) -> bool {
        self.visibility.block(self.navigation.active(), index)
    }

    /// Whether the switch of `row` shows as on.
    pub fn switch_on(
        &self,
        row: &Row,
    ) -> bool {
        let stored = self
            .document
            .value(&row.key)
            .and_then(Value::as_bool)
            .unwrap_or(false);
        let inverted = matches!(row.control, Control::Switch { invert: true });
        stored != inverted
    }

    /// The options of the select `row`.
    pub fn choices(
        &self,
        row: &Row,
    ) -> Vec<Choice> {
        let Control::Select { source, .. } = &row.control else {
            return Vec::new();
        };
        match source {
            ChoiceSource::Fixed(choices) => choices.clone(),
            ChoiceSource::Provided(name) => self.choices.get(name).cloned().unwrap_or_default(),
        }
    }

    /// The index of the option the select `row` currently stores.
    pub fn selected_index(
        &self,
        row: &Row,
    ) -> Option<usize> {
        let stored = self.document.value(&row.key)?.as_text()?;
        self.choices(row)
            .iter()
            .position(|choice| choice.value == stored)
    }

    /// The text of the field `row`: the draft being typed, or the stored value.
    pub fn field_text(
        &self,
        row: &Row,
    ) -> String {
        self.document.text(row)
    }

    /// The problem with what is typed in `row`, if any.
    pub fn issue(
        &self,
        row: &Row,
    ) -> Option<Issue> {
        let text = self.document.text(row);
        row.control.issue(&text)
    }

    /// Whether the field of `row` holds typing that is not stored yet.
    pub fn is_dirty(
        &self,
        row: &Row,
    ) -> bool {
        self.document.is_dirty(row)
    }

    /// Whether a write is still waiting for the backend.
    pub fn has_unsaved(&self) -> bool {
        self.document.has_unsaved()
    }

    /// Whether `row` shows its reset icon: it has a default and holds something else.
    pub fn resettable(
        &self,
        row: &Row,
    ) -> bool {
        row.default.is_some() && !self.document.is_default(row)
    }

    /// The description under the label: the invalid-value text while the field is invalid.
    pub fn description<'a>(
        &self,
        row: &'a Row,
    ) -> Option<&'a str> {
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
    pub fn toggle(
        &mut self,
        row: &Row,
    ) -> Option<Effect> {
        let stored = self.document.value(&row.key).and_then(Value::as_bool)?;
        Some(self.write(&row.key, Value::Bool(!stored)))
    }

    /// Stores the option at `index` of the select `row`.
    pub fn choose(
        &mut self,
        row: &Row,
        index: usize,
    ) -> Option<Effect> {
        let choice = self.choices(row).into_iter().nth(index)?;
        Some(self.write(&row.key, Value::Text(choice.value)))
    }

    /// Records typing in the field of `row` without storing it.
    pub fn edit(
        &mut self,
        row: &Row,
        text: &str,
    ) {
        self.document.edit(&row.key, text);
    }

    /// Stores what is typed in `row` (Enter, or the field losing focus); text that does not parse is dropped; text the validator refuses is stored and flagged.
    pub fn commit(
        &mut self,
        row: &Row,
    ) -> Option<Effect> {
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
    pub fn revert(
        &mut self,
        row: &Row,
    ) {
        self.document.revert(&row.key);
    }

    /// Moves the number of `row` one step.
    pub fn nudge(
        &mut self,
        row: &Row,
        nudge: Nudge,
    ) -> Option<Effect> {
        let Control::Number { range, .. } = &row.control else {
            return None;
        };
        let typed = range.parse(&self.document.text(row));
        let current = typed.or_else(|| self.document.value(&row.key).and_then(Value::as_number))?;
        let next = range.stepped(current, nudge);
        Some(self.write(&row.key, Value::Number(next)))
    }

    /// Restores the default of `row`.
    pub fn reset(
        &mut self,
        row: &Row,
    ) -> Option<Effect> {
        let default = row.default.clone()?;
        if self.document.is_default(row) {
            self.document.revert(&row.key);
            return None;
        }
        Some(self.write(&row.key, default))
    }

    /// Whether the button `spec` can be pressed.
    pub fn button_enabled(
        &self,
        spec: &ButtonSpec,
    ) -> bool {
        spec.enabled
            .as_ref()
            .is_none_or(|rule| rule.holds(&self.scope()))
    }

    /// Presses `spec`: asks its confirmation first, or returns the effect at once.
    pub fn press_button(
        &mut self,
        spec: &ButtonSpec,
    ) -> Option<Effect> {
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
    pub fn confirm_or_run(
        &mut self,
        confirm: Option<&Confirm>,
        effect: Effect,
    ) -> Option<Effect> {
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

    fn write(
        &mut self,
        key: &Key,
        value: Value,
    ) -> Effect {
        let effect = self.document.apply(key, value);
        self.refresh();
        effect
    }

    /// The backend stored `key`.
    pub fn acknowledged(
        &mut self,
        key: &Key,
    ) {
        self.document.acknowledge(key);
    }

    /// The backend refused `key`: its value goes back.
    pub fn write_failed(
        &mut self,
        key: &Key,
    ) {
        self.document.fail(key);
        self.refresh();
    }

    /// The label of the row that stores `key`, or the key itself when no row does.
    pub fn label_of(
        &self,
        key: &Key,
    ) -> String {
        self.schema
            .rows()
            .find(|row| &row.key == key)
            .map_or_else(|| key.to_string(), |row| row.label.clone())
    }

    /// The backend reports a new value for `key`.
    pub fn remote_changed(
        &mut self,
        key: &Key,
        value: Value,
    ) {
        self.document.remote(key, value);
        self.refresh();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::condition::Condition;
    use crate::control_width::ControlWidth;
    use crate::number_range::NumberRange;
    use crate::section::Section;
    use crate::shortcut_group::ShortcutGroup;
    use crate::test_support::{Case, run_cases};
    use crate::validator::Validator;
    use crate::values::Values;

    fn engine_row() -> Row {
        Row::new(
            "engine",
            "Search Engine",
            Control::select(
                &[("duckduckgo", "DuckDuckGo"), ("custom", "Custom")],
                ControlWidth::Default,
            ),
        )
        .default_value("duckduckgo")
    }

    fn custom_row() -> Row {
        Row::new(
            "customUrl",
            "Custom Search URL",
            Control::text("https://kagi.com/search?q=%s", ControlWidth::TextWide)
                .validated(Validator::SearchTemplate),
        )
        .describe("Search address")
        .describe_invalid("Must be an http(s) address containing %s")
        .when(Condition::equals("engine", "custom"))
    }

    fn size_row() -> Row {
        Row::new(
            "fontSize",
            "UI Font Size",
            Control::number(
                NumberRange::new(10.0, 24.0, 0.5).snapping(),
                ControlWidth::Compact,
            ),
        )
        .default_value(15.0)
    }

    fn gitignore_row() -> Row {
        Row::new(
            "showIgnored",
            "Respect .gitignore",
            Control::inverted_switch(),
        )
        .default_value(Value::Bool(true))
    }

    fn clear_button() -> ButtonSpec {
        ButtonSpec::new("Clear data", Action::backend("browser.clearData"))
            .enabled_when(Condition::capability("browser.clearData"))
            .confirm(Confirm::new("Clear?", "Signs you out.", "Clear data"))
    }

    fn schema() -> Schema {
        let appearance = Page::new("appearance", "Appearance", "paint-brush").block(
            Section::new("Typography")
                .headerless()
                .row(size_row())
                .row(gitignore_row()),
        );
        let browser = Page::new("browser", "Browser", "globe")
            .block(
                Section::new("Navigation")
                    .row(engine_row())
                    .row(custom_row()),
            )
            .block(Section::new("Privacy").row(Row::new(
                "clear",
                "Clear Browsing Data",
                Control::Button(clear_button()),
            )));
        let keys = Page::new("keybindings", "Keybindings", "keyboard").block(
            ShortcutGroup::new("Tabs", "overrides")
                .shortcut(Shortcut::new("tabs.new", "New tab", "mod+t"))
                .shortcut(Shortcut::new("tabs.close", "Close active tab", "mod+w")),
        );
        Schema::new(vec![appearance, browser, keys]).unwrap()
    }

    fn state_with(
        values: Values,
        capability: Option<&str>,
    ) -> SettingsState {
        let snapshot = Snapshot::new(values);
        let snapshot = capability.map_or(snapshot.clone(), |name| snapshot.with_capability(name));
        SettingsState::new(schema(), snapshot, None, Primary::Control)
    }

    fn state() -> SettingsState {
        state_with(Values::default(), Some("browser.clearData"))
    }

    fn write(
        key: &str,
        value: impl Into<Value>,
    ) -> Effect {
        Effect::Write(Key::from(key), value.into())
    }

    fn opens_on_the_requested_page_or_the_first() {
        assert_eq!(state().page().map(Page::label), Some("Appearance"));
        let deep = SettingsState::new(
            schema(),
            Snapshot::default(),
            Some(&PageId::from("browser")),
            Primary::Control,
        );
        assert_eq!(deep.page().map(Page::label), Some("Browser"));
        let unknown = SettingsState::new(
            schema(),
            Snapshot::default(),
            Some(&PageId::from("nope")),
            Primary::Control,
        );
        assert_eq!(
            unknown.page().map(Page::label),
            Some("Appearance"),
            "an unknown deep link falls back to the first page"
        );
    }

    fn rail_selection_and_keyboard_movement() {
        let mut state = state();
        state.move_selection(RailMove::Next);
        assert_eq!(state.page().map(Page::label), Some("Browser"));
        state.move_selection(RailMove::Last);
        assert_eq!(state.page().map(Page::label), Some("Keybindings"));
        state.select_page(&PageId::from("appearance"));
        assert_eq!(state.page().map(Page::label), Some("Appearance"));
        assert_eq!(state.visible_pages().len(), 3);
    }

    fn search_filters_pages_and_moves_the_selection_off_a_hidden_page() {
        let mut state = state();
        state.set_query("search engine");
        let labels: Vec<&str> = state
            .visible_pages()
            .iter()
            .map(|page| page.label())
            .collect();
        assert_eq!(labels, ["Browser"]);
        assert_eq!(
            state.page().map(Page::label),
            Some("Browser"),
            "selection follows the only match"
        );
        assert!(state.row_visible(&engine_row()));
        state.set_query("zzz");
        assert_eq!(state.visible_pages().len(), 0);
        assert!(state.page().is_none(), "no match shows no page");
        state.set_query("");
        assert_eq!(state.visible_pages().len(), 3);
        assert_eq!(state.query(), "");
    }

    fn switches_honour_inversion() {
        let mut state = state();
        let row = gitignore_row();
        assert!(!state.switch_on(&row), "stored true reads as off");
        assert_eq!(
            state.toggle(&row),
            Some(write("showIgnored", Value::Bool(false)))
        );
        assert!(state.switch_on(&row), "stored false reads as on");
        assert!(state.resettable(&row));
        assert_eq!(
            state.reset(&row),
            Some(write("showIgnored", Value::Bool(true)))
        );
        assert!(!state.resettable(&row));
    }

    fn selects_store_the_option_value_and_reveal_dependent_rows() {
        let mut state = state();
        let (engine, custom) = (engine_row(), custom_row());
        state.select_page(&PageId::from("browser"));
        assert_eq!(state.selected_index(&engine), Some(0));
        assert!(
            !state.row_visible(&custom),
            "the custom URL row waits for the custom engine"
        );
        assert_eq!(state.choose(&engine, 1), Some(write("engine", "custom")));
        assert_eq!(state.selected_index(&engine), Some(1));
        assert!(state.row_visible(&custom));
        assert_eq!(
            state.choose(&engine, 9),
            None,
            "an index past the options does nothing"
        );
    }

    fn numbers_commit_once_clamped_and_never_while_typing() {
        let mut state = state();
        let row = size_row();
        state.edit(&row, "2");
        assert!(state.is_dirty(&row));
        assert_eq!(state.field_text(&row), "2");
        assert_eq!(
            state.commit(&row),
            Some(write("fontSize", 10.0)),
            "clamped to the minimum on commit"
        );
        assert!(!state.is_dirty(&row));
        assert_eq!(state.field_text(&row), "10");
        state.edit(&row, "abc");
        assert_eq!(state.issue(&row), Some(Issue::NotANumber));
        assert_eq!(state.commit(&row), None);
        assert_eq!(
            state.field_text(&row),
            "10",
            "unparsable text is dropped, the stored value stays"
        );
        state.edit(&row, "10");
        assert_eq!(
            state.commit(&row),
            None,
            "typing the stored value back writes nothing"
        );
    }

    fn steppers_move_from_what_is_typed_and_stop_at_the_bounds() {
        let mut state = state();
        let row = size_row();
        assert_eq!(state.nudge(&row, Nudge::Up), Some(write("fontSize", 15.5)));
        state.edit(&row, "23.5");
        assert_eq!(
            state.nudge(&row, Nudge::Up),
            Some(write("fontSize", 24.0)),
            "starts from the typed value"
        );
        assert_eq!(
            state.nudge(&row, Nudge::Up),
            Some(write("fontSize", 24.0)),
            "the maximum holds"
        );
        assert_eq!(
            state.nudge(&gitignore_row(), Nudge::Up),
            None,
            "only numbers step"
        );
    }

    fn text_is_kept_as_typed_but_flagged_and_describes_itself() {
        let mut state = state_with(
            Values::default().with("engine", "custom"),
            Some("browser.clearData"),
        );
        let row = custom_row();
        assert_eq!(state.description(&row), Some("Search address"));
        state.edit(&row, "not a url");
        assert_eq!(state.issue(&row), Some(Issue::BadSearchTemplate));
        assert_eq!(
            state.description(&row),
            Some("Must be an http(s) address containing %s")
        );
        assert_eq!(state.commit(&row), Some(write("customUrl", "not a url")));
        state.revert(&row);
    }

    fn revert_drops_the_draft() {
        let mut state = state();
        let row = size_row();
        state.edit(&row, "12");
        state.revert(&row);
        assert!(!state.is_dirty(&row));
        assert_eq!(state.field_text(&row), "15");
    }

    fn a_failed_write_restores_the_value_and_names_the_row() {
        let mut state = state();
        let row = gitignore_row();
        assert!(state.toggle(&row).is_some());
        assert!(state.has_unsaved());
        state.write_failed(&Key::from("showIgnored"));
        assert_eq!(
            state.label_of(&Key::from("showIgnored")),
            "Respect .gitignore"
        );
        assert!(!state.switch_on(&row), "back to the default");
        assert!(!state.has_unsaved());
        assert!(state.toggle(&row).is_some());
        state.acknowledged(&Key::from("showIgnored"));
        assert!(!state.has_unsaved());
    }

    fn remote_changes_update_values_and_conditions() {
        let mut state = state();
        state.remote_changed(&Key::from("engine"), Value::from("custom"));
        state.select_page(&PageId::from("browser"));
        assert!(state.row_visible(&custom_row()));
    }

    fn buttons_follow_capabilities_and_confirm_before_running() {
        let mut state = state();
        let spec = clear_button();
        assert!(state.button_enabled(&spec));
        assert_eq!(
            state.press_button(&spec),
            None,
            "the confirmation comes first"
        );
        assert_eq!(state.prompt().map(Prompt::title), Some("Clear?"));
        assert_eq!(
            state.accept_prompt(),
            Some(Effect::Invoke("browser.clearData".to_owned()))
        );
        assert!(state.prompt().is_none());
        state.press_button(&spec);
        state.dismiss_prompt();
        assert!(state.prompt().is_none());
        let mut without = state_with(Values::default(), None);
        assert!(!without.button_enabled(&spec));
        assert_eq!(without.press_button(&spec), None);
        assert!(without.prompt().is_none(), "a disabled button asks nothing");
    }

    fn a_button_without_confirmation_returns_its_effect_at_once() {
        let mut state = state();
        let spec = ButtonSpec::new("Add host", Action::backend("hosts.add"));
        assert_eq!(
            state.press_button(&spec),
            Some(Effect::Invoke("hosts.add".to_owned()))
        );
    }

    const CASES: &[Case] = &[
        (
            "opens on the requested page or the first",
            opens_on_the_requested_page_or_the_first,
        ),
        (
            "rail selection and keyboard movement",
            rail_selection_and_keyboard_movement,
        ),
        (
            "search filters pages and moves the selection off a hidden page",
            search_filters_pages_and_moves_the_selection_off_a_hidden_page,
        ),
        ("switches honour inversion", switches_honour_inversion),
        (
            "selects store the option value and reveal dependent rows",
            selects_store_the_option_value_and_reveal_dependent_rows,
        ),
        (
            "numbers commit once clamped and never while typing",
            numbers_commit_once_clamped_and_never_while_typing,
        ),
        (
            "steppers move from what is typed and stop at the bounds",
            steppers_move_from_what_is_typed_and_stop_at_the_bounds,
        ),
        (
            "text is kept as typed but flagged and describes itself",
            text_is_kept_as_typed_but_flagged_and_describes_itself,
        ),
        ("revert drops the draft", revert_drops_the_draft),
        (
            "a failed write restores the value and names the row",
            a_failed_write_restores_the_value_and_names_the_row,
        ),
        (
            "remote changes update values and conditions",
            remote_changes_update_values_and_conditions,
        ),
        (
            "buttons follow capabilities and confirm before running",
            buttons_follow_capabilities_and_confirm_before_running,
        ),
        (
            "a button without confirmation returns its effect at once",
            a_button_without_confirmation_returns_its_effect_at_once,
        ),
    ];

    #[test]
    fn every_scenario_holds() {
        run_cases(CASES);
    }
}
