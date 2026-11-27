#[derive(Debug, Clone, PartialEq, Eq)]
pub enum MenuEntry {
    Item {
        label: String,
        enabled: bool,
        checked: bool,
    },
    Submenu {
        label: String,
        enabled: bool,
        entries: Vec<MenuEntry>,
    },
    Separator,
}
