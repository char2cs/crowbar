//! The Browser page.

use super::rows::{number, row};
use crate::action::Action;
use crate::button_spec::ButtonSpec;
use crate::condition::Condition;
use crate::confirm::Confirm;
use crate::control::Control;
use crate::control_width::ControlWidth;
use crate::page::Page;
use crate::row::Row;
use crate::section::Section;
use crate::validator::Validator;

const CLEAR_DATA: &str = "browser.clearData";

pub(super) fn page() -> Page {
    Page::new("browser", "Browser", "globe")
        .block(navigation())
        .block(Section::new("Display").row(row(
            ("browserZoom", "Page Zoom"),
            "Default zoom for pages, in percent; applies to tabs opened afterwards",
            number((50.0, 300.0, 10.0), ControlWidth::Compact),
            100.0,
        )))
        .block(Section::new("Privacy").row(clear_data()))
}

fn navigation() -> Section {
    Section::new("Navigation")
        .headerless()
        .row(row(
            ("browserHomepage", "Homepage"),
            "Page a new browser tab opens on; leave empty for a blank address bar",
            Control::text("https://example.com", ControlWidth::TextWide),
            "",
        ))
        .row(row(
            ("browserSearchEngine", "Search Engine"),
            "Used when the address bar text is not a web address",
            Control::select(
                &[
                    ("duckduckgo", "DuckDuckGo"),
                    ("google", "Google"),
                    ("bing", "Bing"),
                    ("brave", "Brave Search"),
                    ("custom", "Custom"),
                ],
                ControlWidth::Default,
            ),
            "duckduckgo",
        ))
        .row(custom_search())
}

fn custom_search() -> Row {
    row(
        ("browserCustomSearchUrl", "Custom Search URL"),
        "Search address for the custom engine, with %s where the query goes",
        Control::text("https://kagi.com/search?q=%s", ControlWidth::TextWide)
            .validated(Validator::SearchTemplate),
        "",
    )
    .describe_invalid("Must be an http(s) address containing %s; DuckDuckGo is used until it is")
    .when(Condition::equals("browserSearchEngine", "custom"))
}

fn clear_data() -> Row {
    let spec = ButtonSpec::new("Clear data", Action::backend(CLEAR_DATA))
        .enabled_when(Condition::capability(CLEAR_DATA))
        .disabled_note("Not available on this system: browser tabs share their storage with the app itself, so clearing it would erase app data too.")
        .confirm(Confirm::new(
            "Clear browsing data?",
            "This signs you out of every site and deletes cached files and site storage of all browser tabs. Open pages keep what they already loaded until they reload.",
            "Clear data",
        ));
    Row::new(
        "browserClearData",
        "Clear Browsing Data",
        Control::Button(spec),
    )
    .describe("Delete cookies, cache and site storage of every browser tab")
}

#[cfg(test)]
#[path = "browser_tests.rs"]
mod tests;
