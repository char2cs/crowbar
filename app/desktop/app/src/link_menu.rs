use gpui::{Entity, Pixels, Point};
use ui::Menu;

pub(crate) struct LinkMenu {
    pub(crate) menu: Entity<Menu>,
    pub(crate) at: Point<Pixels>,
}
