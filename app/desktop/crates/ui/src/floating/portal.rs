use std::rc::Rc;

use gpui::{App, Bounds, ElementId, IntoElement, RenderOnce, Styled, Window, canvas};

use crate::floating::layers::{Owners, place};
use crate::floating::portal::cover::covering::covering;
use crate::floating::portal::cover::opening::Opening;
use crate::floating::portal::reach::Reach;
use crate::floating::portal::slot::keyed;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::layer as paint_order;

pub(crate) mod cover;
pub(crate) mod float;
pub(crate) mod layer;
pub(crate) mod reach;
pub(crate) mod slot;

use cover::Away;
pub use cover::Cover;
pub use float::Float;
use layer::{Build, Layer};

#[derive(IntoElement)]
pub struct Portal {
    id: ElementId,
    reach: Reach,
    priority: usize,
    float: Float,
    away: Option<Away>,
    build: Build,
}

impl Portal {
    pub fn here<E: IntoElement>(
        id: impl Into<ElementId>,
        build: impl Fn(&mut Window, &mut App) -> E + 'static,
    ) -> Self {
        Self::new(id.into(), Reach::Here, build)
    }

    pub fn lifted<E: IntoElement>(
        id: impl Into<ElementId>,
        build: impl Fn(&mut Window, &mut App) -> E + 'static,
    ) -> Self {
        Self::new(id.into(), Reach::Lifted, build)
    }

    pub fn window<E: IntoElement>(
        id: impl Into<ElementId>,
        build: impl Fn(&mut Window, &mut App) -> E + 'static,
    ) -> Self {
        Self::new(id.into(), Reach::Window, build)
    }

    pub fn cover<E: IntoElement>(
        id: impl Into<ElementId>,
        build: impl Fn(&mut Window, &mut App) -> E + 'static,
    ) -> Self {
        Self::new(id.into(), Reach::Cover, build)
    }

    fn new<E: IntoElement>(
        id: ElementId,
        reach: Reach,
        build: impl Fn(&mut Window, &mut App) -> E + 'static,
    ) -> Self {
        let build: Build = Rc::new(move |window, cx| build(window, cx).into_any_element());
        let float = match reach {
            Reach::Lifted => Float::Drag,
            Reach::Here | Reach::Window | Reach::Cover => Float::Passive,
        };
        Self {
            id,
            reach,
            priority: paint_order::OVERLAY,
            float,
            away: None,
            build,
        }
    }

    #[must_use]
    pub(crate) fn away(
        mut self,
        away: Away,
    ) -> Self {
        self.away = Some(away);
        self
    }

    #[must_use]
    pub fn float(
        mut self,
        float: Float,
    ) -> Self {
        self.float = float;
        self
    }

    #[must_use]
    pub fn priority(
        mut self,
        priority: usize,
    ) -> Self {
        self.priority = priority;
        self
    }

    fn layer(&self) -> Layer {
        Layer {
            reach: self.reach,
            priority: self.priority,
            build: Rc::clone(&self.build),
            bounds: Bounds::default(),
        }
    }
}

impl RenderOnce for Portal {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let layer = self.layer();
        if self.float != Float::Passive {
            let opening = Opening(self.id, self.float, self.away);
            return covering(opening, &layer, window, cx);
        }
        let Some(owner) = Owners::of(window, cx) else {
            return window.with_element_namespace(self.id, |window| layer.inline(window, cx));
        };
        let held = owner.downgrade();
        let release = move |slot, cx: &mut App| Owners::hide(&held, slot, cx);
        let key = keyed(self.id, release, (window, cx));
        canvas(
            move |bounds, _, cx| place(&owner, (key, Layer { bounds, ..layer }), cx),
            |_, (), _, _| {},
        )
        .absolute()
        .size_full()
        .into_any_element()
    }
}

opaque_debug!(Portal);

#[cfg(test)]
mod tests {
    use gpui::{IntoElement, ParentElement, Styled, TestAppContext, div, point, px};

    use super::*;
    use crate::test_support::floats::floated;
    use crate::test_support::probe::open_floating;
    use crate::testing::newest_window;

    fn trigger(portal: Portal) -> impl IntoElement {
        div()
            .relative()
            .w(px(40.0))
            .h(px(20.0))
            .child(div().absolute().top_full().left_0().child(portal))
    }

    #[gpui::test]
    fn a_portal_paints_in_a_float_window_over_its_owner_and_not_in_the_owner(
        cx: &mut TestAppContext
    ) {
        let mut owner = open_floating(cx, |_, _| trigger(Portal::here("p", |_, _| floated())));
        let mut top = newest_window(&mut owner);
        assert_eq!(owner.debug_bounds("floated"), None, "nothing in the owner");
        let painted = top.debug_bounds("floated").unwrap_or_default();
        let anchor = owner.debug_bounds("probe").unwrap_or_default();
        assert_eq!(painted.origin, point(anchor.left(), anchor.bottom()));
    }

    #[gpui::test]
    fn without_a_float_host_the_window_paints_the_portal_itself(cx: &mut TestAppContext) {
        let cx = crate::test_support::probe::open(cx, |_, _| {
            trigger(Portal::here("p", |_, _| floated()))
        });
        assert!(cx.debug_bounds("floated").is_some());
    }
}
