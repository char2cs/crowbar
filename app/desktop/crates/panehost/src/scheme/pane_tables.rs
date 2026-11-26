//! What the scheme and the pane host share.

use crate::scheme::menu::pane_menus::PaneMenus;
use crate::scheme::origin_table::OriginTable;

/// The tables a [`PaneProtocol`](crate::PaneProtocol) and its [`PaneHost`](crate::PaneHost)
/// share: the host writes which pane each web view acts for and pops the menus pages ask for;
/// the protocol reads the one and sends the other.
#[derive(Debug, Clone, Default)]
pub struct PaneTables {
    /// Which pane and origin each web view acts for.
    pub origins: OriginTable,
    /// The native menus pages ask for.
    pub menus: PaneMenus,
}
