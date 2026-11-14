use std::collections::BTreeSet;

use super::*;
use crate::condition::Condition;
use crate::control::Control;
use crate::heading::Heading;
use crate::values::Values;

fn schema() -> Schema {
    let appearance = Page::new("appearance", "Appearance", "paint-brush").block(
        Section::new("Typography")
            .row(
                Row::new("uiFontSize", "UI Font Size", Control::switch())
                    .describe("Adjust UI text")
                    .keywords(&["zoom"]),
            )
            .row(Row::new(
                "sidebarPosition",
                "Sidebar Position",
                Control::switch(),
            )),
    );
    let browser = Page::new("browser", "Browser", "globe").block(
        Section::new("Navigation")
            .row(Row::new("browserHomepage", "Homepage", Control::switch()))
            .row(
                Row::new("customUrl", "Custom Search URL", Control::switch())
                    .when(Condition::equals("engine", "custom")),
            ),
    );
    let keys = Page::new("keybindings", "Keybindings", "keyboard").block(
        ShortcutGroup::new("Tabs", "overrides")
            .shortcut(Shortcut::new("tabs.new", "New tab", "mod+t")),
    );
    let hosts = Page::new("hosts", "Hosts", "hard-drives")
        .block(Heading::new("Hosts", "Add another machine."));
    let hidden = Page::new("dev", "Developer", "bug").when(Condition::capability("dev"));
    Schema::new(vec![appearance, browser, keys, hosts, hidden]).unwrap()
}

fn compute(schema: &Schema, query: &str) -> Visibility {
    let values = Values::default();
    let capabilities = BTreeSet::new();
    let scope = Scope {
        values: &values,
        defaults: schema.defaults(),
        capabilities: &capabilities,
    };
    Visibility::compute(schema, &scope, query)
}

fn page_ids(visibility: &Visibility) -> Vec<&str> {
    visibility.pages().iter().map(PageId::as_str).collect()
}

#[test]
fn without_a_query_every_applicable_page_and_row_shows() {
    let schema = schema();
    let visibility = compute(&schema, "  ");
    assert_eq!(
        page_ids(&visibility),
        ["appearance", "browser", "keybindings", "hosts"]
    );
    let shown: Vec<&str> = schema
        .rows()
        .filter(|row| visibility.row(row))
        .map(|row| row.key.as_str())
        .collect();
    assert_eq!(
        shown,
        ["uiFontSize", "sidebarPosition", "browserHomepage"],
        "the conditional row is hidden"
    );
}

#[test]
fn one_matcher_decides_pages_and_rows_together() {
    type Case = (
        &'static str,
        &'static [&'static str],
        &'static [&'static str],
    );
    const CASES: &[Case] = &[
        ("font", &["appearance"], &["uiFontSize"]),
        ("zoom", &["appearance"], &["uiFontSize"]),
        ("adjust text", &["appearance"], &["uiFontSize"]),
        (
            "typography",
            &["appearance"],
            &["uiFontSize", "sidebarPosition"],
        ),
        ("browser", &["browser"], &["browserHomepage"]),
        ("zzz", &[], &[]),
    ];
    let schema = schema();
    for &(query, pages, rows) in CASES {
        let visibility = compute(&schema, query);
        assert_eq!(page_ids(&visibility), pages, "pages for {query:?}");
        let shown: Vec<&str> = schema
            .rows()
            .filter(|row| visibility.row(row))
            .map(|row| row.key.as_str())
            .collect();
        assert_eq!(shown, rows, "rows for {query:?}");
    }
}

#[test]
fn shortcuts_and_heading_pages_are_searchable_too() {
    let schema = schema();
    let by_label = compute(&schema, "new tab");
    assert_eq!(page_ids(&by_label), ["keybindings"]);
    assert!(
        schema
            .shortcuts()
            .all(|(_, shortcut)| by_label.shortcut(shortcut))
    );
    let by_text = compute(&schema, "another machine");
    assert_eq!(page_ids(&by_text), ["hosts"]);
    assert!(by_text.block(&PageId::from("hosts"), 0));
    let by_page = compute(&schema, "hosts");
    assert!(
        by_page.block(&PageId::from("hosts"), 0),
        "a matching page label shows all its blocks"
    );
}
