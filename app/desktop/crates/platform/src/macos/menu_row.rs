#[derive(Debug, Clone, PartialEq, Eq)]
pub enum MenuRow {
    Item {
        label: String,
        enabled: bool,
        checked: bool,
    },
    Submenu {
        label: String,
        enabled: bool,
        rows: Vec<MenuRow>,
    },
    Separator,
}
