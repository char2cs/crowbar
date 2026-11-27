use crate::keys::Shortcut;
use crate::keys::ShortcutGroup;
use crate::schema::Action;
use crate::schema::ButtonSpec;
use crate::schema::Condition;
use crate::schema::Confirm;
use crate::schema::Control;
use crate::schema::ControlWidth;
use crate::schema::NumberRange;
use crate::schema::Page;
use crate::schema::Row;
use crate::schema::Schema;
use crate::schema::SearchEntry;
use crate::schema::Section;
use crate::schema::Validator;
use crate::state::State;
use crate::state::effect::Effect;
use crate::values::Key;
use crate::values::Snapshot;
use crate::values::Value;
use crate::values::Values;
use chord::Primary;

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
    let appearance = Page::new("appearance", "Appearance", "paint-brush")
        .indexed([
            SearchEntry::new("Typography", "UI Font Size", "Adjust UI text").keywords(&["font"]),
        ])
        .block(
            Section::new("Typography")
                .headerless()
                .row(size_row())
                .row(gitignore_row()),
        );
    let browser = Page::new("browser", "Browser", "globe")
        .indexed([
            SearchEntry::new("Navigation", "Search Engine", "Used for the address bar")
                .keywords(&["font"]),
            SearchEntry::new("Privacy", "Clear Browsing Data", "Delete cookies"),
        ])
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
) -> State {
    let snapshot = Snapshot::new(values);
    let snapshot = capability.map_or(snapshot.clone(), |name| snapshot.with_capability(name));
    State::new(schema(), snapshot, None, Primary::Control)
}

pub(crate) fn state() -> State {
    state_with(Values::default(), Some("browser.clearData"))
}

pub(crate) fn write(
    key: &str,
    value: impl Into<Value>,
) -> Effect {
    Effect::Write(Key::from(key), value.into())
}
