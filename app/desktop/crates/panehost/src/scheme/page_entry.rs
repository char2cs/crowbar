//! One menu entry as a page writes it.

use serde::Deserialize;
use webview::MenuEntry;

#[derive(Debug, Deserialize)]
#[serde(tag = "kind", rename_all = "lowercase")]
pub(crate) enum PageEntry {
    Item {
        label: String,
        enabled: bool,
    },
    Submenu {
        label: String,
        enabled: bool,
        entries: Vec<PageEntry>,
    },
    Separator,
}

impl PageEntry {
    pub(crate) fn into_menu(self) -> MenuEntry {
        match self {
            Self::Item { label, enabled } => MenuEntry::Item {
                label,
                enabled,
                checked: false,
            },
            Self::Submenu {
                label,
                enabled,
                entries,
            } => MenuEntry::Submenu {
                label,
                enabled,
                entries: entries.into_iter().map(Self::into_menu).collect(),
            },
            Self::Separator => MenuEntry::Separator,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_page_s_entries_become_plain_menu_entries_without_checks() {
        let page = r#"[{"kind":"item","label":"Delete","enabled":false},{"kind":"separator"},
            {"kind":"submenu","label":"Align","enabled":true,"entries":[{"kind":"item","label":"Left","enabled":true}]}]"#;
        let entries: Vec<PageEntry> = serde_json::from_str(page).unwrap();
        let menu: Vec<MenuEntry> = entries.into_iter().map(PageEntry::into_menu).collect();
        let item = |label: &str, enabled| MenuEntry::Item {
            label: label.into(),
            enabled,
            checked: false,
        };
        let align = MenuEntry::Submenu {
            label: "Align".into(),
            enabled: true,
            entries: vec![item("Left", true)],
        };
        assert_eq!(menu, [item("Delete", false), MenuEntry::Separator, align]);
    }
}
