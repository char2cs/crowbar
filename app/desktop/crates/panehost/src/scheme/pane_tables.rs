use crate::scheme::menu::pane_menus::PaneMenus;
use crate::scheme::origin_table::OriginTable;

#[derive(Debug, Clone, Default)]
pub struct PaneTables {
    pub origins: OriginTable,
    pub menus: PaneMenus,
}
