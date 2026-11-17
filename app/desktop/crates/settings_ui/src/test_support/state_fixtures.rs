//! Schema and state fixtures shared by the state tests.

use crate::action::Action;
use crate::button_spec::ButtonSpec;
use crate::condition::Condition;
use crate::confirm::Confirm;
use crate::control::Control;
use crate::control_width::ControlWidth;
use crate::effect::Effect;
use crate::key::Key;
use crate::number_range::NumberRange;
use crate::page::Page;
use crate::primary::Primary;
use crate::row::Row;
use crate::schema::Schema;
use crate::section::Section;
use crate::settings_state::SettingsState;
use crate::shortcut::Shortcut;
use crate::shortcut_group::ShortcutGroup;
use crate::snapshot::Snapshot;
use crate::validator::Validator;
use crate::value::Value;
use crate::values::Values;

pub(crate) fn engine_row() -> Row {
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

pub(crate) fn custom_row() -> Row {
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

pub(crate) fn size_row() -> Row {
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

pub(crate) fn gitignore_row() -> Row {
    Row::new(
        "showIgnored",
        "Respect .gitignore",
        Control::inverted_switch(),
    )
    .default_value(Value::Bool(true))
}

pub(crate) fn clear_button() -> ButtonSpec {
    ButtonSpec::new("Clear data", Action::backend("browser.clearData"))
        .enabled_when(Condition::capability("browser.clearData"))
        .confirm(Confirm::new("Clear?", "Signs you out.", "Clear data"))
}

pub(crate) fn schema() -> Schema {
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

pub(crate) fn state_with(
    values: Values,
    capability: Option<&str>,
) -> SettingsState {
    let snapshot = Snapshot::new(values);
    let snapshot = capability.map_or(snapshot.clone(), |name| snapshot.with_capability(name));
    SettingsState::new(schema(), snapshot, None, Primary::Control)
}

pub(crate) fn state() -> SettingsState {
    state_with(Values::default(), Some("browser.clearData"))
}

pub(crate) fn write(
    key: &str,
    value: impl Into<Value>,
) -> Effect {
    Effect::Write(Key::from(key), value.into())
}
