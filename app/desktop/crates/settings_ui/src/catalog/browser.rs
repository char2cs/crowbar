//! The Browser page.

use super::number_setting::NumberSetting;
use super::rows::row;
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

const PAGE_ZOOM: NumberSetting = NumberSetting::new((50.0, 300.0, 10.0), 100.0);

pub(super) fn page() -> Page {
    Page::new("browser", "Browser", "globe")
        .block(navigation())
        .block(Section::new("Display").row(row(
            ("browserZoom", "Page Zoom"),
            "Default zoom for pages, in percent; applies to tabs opened afterwards",
            PAGE_ZOOM.plain(ControlWidth::Compact),
            PAGE_ZOOM.default,
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
mod tests {
    use super::*;

    #[test]
    fn the_custom_url_row_waits_for_the_custom_engine_and_clear_data_has_a_capability() {
        let page = page();
        let custom = page
            .rows()
            .find(|row| row.key().as_str() == "browserCustomSearchUrl");
        assert!(custom.is_some_and(|row| row.when.is_some() && row.invalid_description.is_some()));
        let clear = page
            .rows()
            .find(|row| row.key().as_str() == "browserClearData");
        let Some(Control::Button(spec)) = clear.map(|row| &row.control) else {
            panic!("the privacy row holds a button");
        };
        assert_eq!(
            spec.enabled,
            Some(Condition::capability("browser.clearData"))
        );
        assert!(spec.confirm.is_some() && spec.disabled_note.is_some());
    }
}
