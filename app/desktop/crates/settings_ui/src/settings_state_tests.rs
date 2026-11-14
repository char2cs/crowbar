use super::*;
use crate::condition::Condition;
use crate::control_width::ControlWidth;
use crate::modifier::Modifier;
use crate::number_range::NumberRange;
use crate::section::Section;
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
    .default_value(true)
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

fn state_with(values: Values, capability: Option<&str>) -> SettingsState {
    let snapshot = Snapshot::new(values);
    let snapshot = capability.map_or(snapshot.clone(), |name| snapshot.with_capability(name));
    SettingsState::new(schema(), snapshot, None, Primary::Control)
}

fn state() -> SettingsState {
    state_with(Values::default(), Some("browser.clearData"))
}

fn write(key: &str, value: impl Into<Value>) -> Effect {
    Effect::Write(Key::from(key), value.into())
}

#[test]
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

#[test]
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

#[test]
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

#[test]
fn switches_honour_inversion() {
    let mut state = state();
    let row = gitignore_row();
    assert!(!state.switch_on(&row), "stored true reads as off");
    assert_eq!(state.toggle(&row), Some(write("showIgnored", false)));
    assert!(state.switch_on(&row), "stored false reads as on");
    assert!(state.resettable(&row));
    assert_eq!(state.reset(&row), Some(write("showIgnored", true)));
    assert!(!state.resettable(&row));
}

#[test]
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

#[test]
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

#[test]
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

#[test]
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

#[test]
fn revert_drops_the_draft() {
    let mut state = state();
    let row = size_row();
    state.edit(&row, "12");
    state.revert(&row);
    assert!(!state.is_dirty(&row));
    assert_eq!(state.field_text(&row), "15");
}

#[test]
fn a_failed_write_restores_the_value_and_names_the_row() {
    let mut state = state();
    let row = gitignore_row();
    assert!(state.toggle(&row).is_some());
    assert!(state.has_unsaved());
    assert_eq!(
        state.write_failed(&Key::from("showIgnored")).as_deref(),
        Some("Respect .gitignore")
    );
    assert!(!state.switch_on(&row), "back to the default");
    assert!(!state.has_unsaved());
    assert!(state.toggle(&row).is_some());
    state.acknowledged(&Key::from("showIgnored"));
    assert!(!state.has_unsaved());
}

#[test]
fn remote_changes_update_values_and_conditions() {
    let mut state = state();
    state.remote_changed(&Key::from("engine"), Value::from("custom"));
    state.select_page(&PageId::from("browser"));
    assert!(state.row_visible(&custom_row()));
}

#[test]
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

#[test]
fn a_button_without_confirmation_returns_its_effect_at_once() {
    let mut state = state();
    let spec = ButtonSpec::new("Add host", Action::backend("hosts.add"));
    assert_eq!(
        state.press_button(&spec),
        Some(Effect::Invoke("hosts.add".to_owned()))
    );
}

fn pressed(key: &str, held: &[Modifier]) -> Pressed {
    Pressed::new(key, held)
}

fn overrides(pairs: &[(&str, &str)]) -> Effect {
    let map = pairs
        .iter()
        .map(|&(command, chord)| (command.to_owned(), chord.to_owned()))
        .collect();
    write("overrides", Value::Map(map))
}

fn group_of(state: &SettingsState) -> (ShortcutGroup, Shortcut) {
    let (group, shortcut) = state.schema().shortcuts().next().unwrap();
    (group.clone(), shortcut.clone())
}

#[test]
fn capture_stores_a_chord_and_refuses_bare_keys() {
    let mut state = state();
    state.toggle_capture("tabs.new");
    assert_eq!(state.capturing(), Some("tabs.new"));
    assert_eq!(state.press_key(&pressed("a", &[])), None);
    assert_eq!(state.capture_refusal(), Some(Refusal::BareKey));
    assert_eq!(
        state.press_key(&pressed("n", &[Modifier::Control, Modifier::Shift])),
        Some(overrides(&[("tabs.new", "mod+shift+n")]))
    );
    assert_eq!(state.capturing(), None);
    let (group, shortcut) = group_of(&state);
    let (chord, source) = state.chord(&group, &shortcut);
    assert_eq!(
        chord.map(|chord| state.label(&chord)),
        Some("Ctrl+Shift+N".to_owned())
    );
    assert_eq!(source, ChordSource::Custom);
    assert!(state.has_custom_chords());
}

#[test]
fn escape_leaves_capture_and_the_default_chord_is_not_stored_as_custom() {
    let mut state = state();
    state.toggle_capture("tabs.new");
    assert_eq!(state.press_key(&pressed("escape", &[])), None);
    assert_eq!(state.capturing(), None);
    state.toggle_capture("tabs.new");
    assert_eq!(
        state.press_key(&pressed("t", &[Modifier::Control])),
        Some(overrides(&[])),
        "the default chord clears the override"
    );
    assert!(!state.has_custom_chords());
}

#[test]
fn conflicts_clear_and_reset_all() {
    let mut state = state();
    state.toggle_capture("tabs.close");
    state.press_key(&pressed("t", &[Modifier::Control, Modifier::Shift]));
    state.toggle_capture("tabs.new");
    state.press_key(&pressed("t", &[Modifier::Control, Modifier::Shift]));
    let (group, shortcut) = group_of(&state);
    assert_eq!(
        state.conflicts(&group, &shortcut),
        ["Close active tab"],
        "the same chord is flagged"
    );
    assert_eq!(
        state.clear_chord("tabs.new"),
        Some(overrides(&[("tabs.close", "mod+shift+t")]))
    );
    let reset = ButtonSpec::new("Reset all", Action::ResetOverrides(Key::from("overrides")));
    assert_eq!(state.press_button(&reset), Some(overrides(&[])));
    assert_eq!(
        state.press_button(&reset),
        None,
        "nothing custom is left to reset"
    );
    assert_eq!(state.clear_chord("unknown.command"), None);
}
