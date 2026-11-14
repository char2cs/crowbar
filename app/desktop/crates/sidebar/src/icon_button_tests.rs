use std::cell::Cell;
use std::rc::Rc;

use gpui::{Context, IntoElement, Render, TestAppContext, Window, point, px};
use ui::IconName;

use super::*;

struct Host {
    clicks: Rc<Cell<u32>>,
}

impl Render for Host {
    fn render(&mut self, _: &mut Window, _: &mut Context<'_, Self>) -> impl IntoElement {
        let clicks = Rc::clone(&self.clicks);
        IconButton::new("button", IconName::Plus)
            .on_click(move |_, _, _| clicks.set(clicks.get() + 1))
    }
}

#[gpui::test]
fn a_click_runs_the_handler(cx: &mut TestAppContext) {
    cx.update(|cx| ui::init(cx).expect("ui installs"));
    let clicks = Rc::new(Cell::new(0));
    let shared = Rc::clone(&clicks);
    let (_, cx) = cx.add_window_view(|_, _| Host { clicks: shared });
    cx.simulate_click(point(px(4.0), px(4.0)), gpui::Modifiers::none());
    assert_eq!(clicks.get(), 1);
}

#[test]
fn the_tile_shape_rounds_more_than_the_small_one() {
    let small = IconButton::new("a", IconName::Plus);
    let tile = IconButton::new("b", IconName::Plus).shape(TileShape::Tile);
    assert!(tile.radius() > small.radius());
}
