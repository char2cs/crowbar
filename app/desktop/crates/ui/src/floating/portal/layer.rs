//! One floating element, painted above the rest of its window.

use std::rc::Rc;

use gpui::{
    AnyElement, App, Bounds, IntoElement, ParentElement, Pixels, Point, Styled, Window, anchored,
    canvas, deferred, div, point, px,
};

use crate::floating::portal::reach::Reach;

pub(crate) type Build = Rc<dyn Fn(Bounds<Pixels>, &mut Window, &mut App) -> AnyElement>;

#[derive(Clone)]
pub(crate) struct Layer {
    pub(crate) reach: Reach,
    pub(crate) priority: usize,
    pub(crate) build: Build,
}

impl Layer {
    pub(crate) fn inline(
        &self,
        window: &mut Window,
        cx: &mut App,
    ) -> AnyElement {
        if let Reach::Here = self.reach {
            return self.deferred_here();
        }
        let content = (self.build)(whole(window), window, cx);
        if let Reach::Lifted = self.reach {
            return content;
        }
        let viewport = window.viewport_size();
        let cover = div()
            .relative()
            .w(viewport.width)
            .h(viewport.height)
            .child(content);
        let cover = anchored().position(window_origin()).child(cover);
        if let Reach::Cover = self.reach {
            return cover.into_any_element();
        }
        deferred(cover)
            .with_priority(self.priority)
            .into_any_element()
    }
}

impl Layer {
    fn deferred_here(&self) -> AnyElement {
        let (build, priority) = (Rc::clone(&self.build), self.priority);
        canvas(
            move |bounds: Bounds<Pixels>, window, cx| {
                let sized = div()
                    .w(bounds.size.width)
                    .h(bounds.size.height)
                    .child(build(bounds, window, cx));
                let mut content = deferred(sized).with_priority(priority).into_any_element();
                content.prepaint_as_root(bounds.origin, bounds.size.into(), window, cx);
                content
            },
            |_, mut content, window, cx| content.paint(window, cx),
        )
        .absolute()
        .size_full()
        .into_any_element()
    }
}

fn whole(window: &Window) -> Bounds<Pixels> {
    Bounds::new(window_origin(), window.viewport_size())
}

fn window_origin() -> Point<Pixels> {
    point(px(0.0), px(0.0))
}

#[cfg(test)]
mod tests {
    use gpui::{InteractiveElement, TestAppContext};

    use super::*;
    use crate::test_support::probe::open;

    const OFFSET: f32 = 24.0;

    fn layer(reach: Reach) -> Layer {
        let build: Build = Rc::new(|_, _, _| {
            div()
                .w(px(30.0))
                .h(px(10.0))
                .debug_selector(|| "floated".into())
                .into_any_element()
        });
        Layer {
            reach,
            priority: 0,
            build,
        }
    }

    fn slotted(
        reach: Reach,
        window: &mut Window,
        cx: &mut App,
    ) -> gpui::Div {
        let slot = div()
            .relative()
            .w(px(40.0))
            .h(px(20.0))
            .child(layer(reach).inline(window, cx));
        div().pl(px(OFFSET)).pt(px(OFFSET)).child(slot)
    }

    #[gpui::test]
    fn each_reach_paints_the_layer_where_it_says(cx: &mut TestAppContext) {
        let cases = [
            ("here, at its slot", Reach::Here, OFFSET),
            ("lifted, at its slot", Reach::Lifted, OFFSET),
            ("window, from the window origin", Reach::Window, 0.0),
            ("cover, from the window origin", Reach::Cover, 0.0),
        ];
        for (case, reach, expected) in cases {
            let window = open(cx, move |window, cx| slotted(reach, window, cx));
            let painted = window.debug_bounds("floated").unwrap_or_default();
            assert_eq!(painted.origin, point(px(expected), px(expected)), "{case}");
        }
    }
}
