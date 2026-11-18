//! What a dropdown opens: the rows and the minimum width.

use std::rc::Rc;

use gpui::{App, AppContext as _, Entity, Pixels, Window};

use crate::floating::menu::Menu;
use crate::floating::menu::menu_item::MenuItem;

#[derive(Clone)]
pub(crate) struct Spec {
    pub(crate) items: Rc<[MenuItem]>,
    pub(crate) min_width: Option<Pixels>,
}

impl Spec {
    pub(crate) fn new(items: Rc<[MenuItem]>) -> Self {
        Self {
            items,
            min_width: None,
        }
    }

    pub(crate) fn build(
        &self,
        window: &mut Window,
        cx: &mut App,
    ) -> Entity<Menu> {
        cx.new(|cx| {
            let menu = Menu::new(Rc::clone(&self.items), window, cx);
            let width = self.min_width.unwrap_or(Menu::DEFAULT_MIN_WIDTH);
            menu.min_width(width)
        })
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, px};

    use super::*;
    use crate::test_support::probe::open;

    fn spec(min_width: Option<Pixels>) -> Spec {
        Spec {
            items: Rc::from(Vec::<MenuItem>::new()),
            min_width,
        }
    }

    #[gpui::test]
    fn menu_is_at_least_the_requested_width(cx: &mut TestAppContext) {
        let wide = spec(Some(px(300.0)));
        let cx = open(cx, move |window, cx| wide.build(window, cx));
        let menu = cx.debug_bounds("ui-menu").unwrap_or_default();
        assert!(f32::from(menu.size.width) >= 300.0, "{menu:?}");
    }

    #[gpui::test]
    fn default_minimum_applies_without_a_request(cx: &mut TestAppContext) {
        let plain = spec(None);
        let cx = open(cx, move |window, cx| plain.build(window, cx));
        let menu = cx.debug_bounds("ui-menu").unwrap_or_default();
        assert!(f32::from(menu.size.width) >= 128.0, "{menu:?}");
    }
}
