//! A floating element to look for, and a portal a test switches off.

use std::cell::Cell;
use std::rc::Rc;

use gpui::{
    Div, InteractiveElement as _, ParentElement as _, Styled as _, TestAppContext,
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
        let portal = showing
            .get()
            .then(|| Portal::here("p", |_, _| floated()).float(float));
        div().children(portal)
    });
    (shown, owner)
}

pub(crate) fn windows(owner: &mut VisualTestContext) -> usize {
    owner.update(|_, cx| cx.windows().len())
}
