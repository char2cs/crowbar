use std::cell::Cell;
use std::rc::Rc;

use gpui::{
    Bounds, Div, InteractiveElement as _, ParentElement as _, Pixels, Styled as _, TestAppContext,
    VisualTestContext, div, px,
};

use crate::test_support::probe::open_floating;
use crate::{Float, Portal};

pub(crate) fn floated() -> Div {
    div()
        .w(px(30.0))
        .h(px(10.0))
        .debug_selector(|| "floated".into())
}

pub(crate) fn toggled(
    cx: &mut TestAppContext,
    float: Float,
) -> (Rc<Cell<bool>>, VisualTestContext) {
    let shown = Rc::new(Cell::new(true));
    let showing = Rc::clone(&shown);
    let owner = open_floating(cx, move |_, _| {
        div().children(showing.get().then(|| solid_portal(float)))
    });
    (shown, owner)
}

fn solid_portal(float: Float) -> Portal {
    Portal::here("p", |_, _| crate::Shaped::solid(floated(), px(0.0))).float(float)
}

pub(crate) fn windows(owner: &mut VisualTestContext) -> usize {
    owner.update(|_, cx| cx.windows().len())
}

pub(crate) fn covered(window: &mut VisualTestContext) -> Vec<Bounds<Pixels>> {
    crate::testing::covered(window).1
}
