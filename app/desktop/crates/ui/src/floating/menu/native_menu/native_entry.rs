use gpui::SharedString;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum NativeEntry {
    Item {
        label: SharedString,
        enabled: bool,
        checked: bool,
    },
    Submenu {
        label: SharedString,
        enabled: bool,
        entries: Vec<NativeEntry>,
    },
    Separator,
}
