use gpui::SharedString;
use sidebar::{Expansion, Row};
use ui::Palette;

use crate::sidebar::RowHandler;

pub(crate) struct ClusterSpec<'a> {
    pub(crate) row: &'a Row,
    pub(crate) group: SharedString,
    pub(crate) expansion: Expansion,
    pub(crate) palette: &'static Palette,
    pub(crate) handler: RowHandler,
}
