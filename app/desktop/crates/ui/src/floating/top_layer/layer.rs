//! One floating element painted by the top layer.

use std::rc::Rc;

use gpui::{
    AnyElement, App, Bounds, IntoElement, ParentElement, Pixels, Point, Styled, Window, anchored,
    canvas, deferred, div, point, px,
};

use crate::floating::top_layer::reach::Reach;

pub(crate) type Build = Rc<dyn Fn(Bounds<Pixels>, &mut Window, &mut App) -> AnyElement>;

#[derive(Clone)]
pub(crate) struct Layer {
    pub(crate) reach: Reach,
    pub(crate) priority: usize,
    pub(crate) build: Build,
}

impl Layer {
    pub(crate) fn paint(
        &self,
        window: &mut Window,
        cx: &mut App,
    ) -> AnyElement {
        let Reach::Here(bounds) = self.reach else {
            let content = (self.build)(whole(window), window, cx);
            return div().absolute().inset_0().child(content).into_any_element();
        };
        let content = (self.build)(bounds, window, cx);
        div()
            .absolute()
            .left(bounds.origin.x)
            .top(bounds.origin.y)
            .w(bounds.size.width)
            .h(bounds.size.height)
            .child(content)
            .into_any_element()
    }

    pub(crate) fn inline(
        &self,
        window: &mut Window,
        cx: &mut App,
    ) -> AnyElement {
        if let Reach::Here(_) = self.reach {
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
    use gpui::{Bounds, InteractiveElement, TestAppContext, size};

    use super::*;
    use crate::test_support::probe::open;

    fn content() -> gpui::Div {
        div().size_full().debug_selector(|| "content".into())
    }

    #[gpui::test]
    fn a_layer_from_a_portal_in_place_is_painted_at_the_portal_bounds(cx: &mut TestAppContext) {
        let bounds = Bounds::new(point(px(30.0), px(20.0)), size(px(10.0), px(10.0)));
        let layer = Layer {
            reach: Reach::Here(bounds),
            priority: 1,
            build: Rc::new(|_, _, _| content().into_any_element()),
        };
        let window = open(cx, move |window, cx| {
            div()
                .size(px(100.0))
                .relative()
                .child(layer.paint(window, cx))
        });
        let painted = window.debug_bounds("content").unwrap_or_default();
        assert_eq!(painted.origin, bounds.origin);
        assert_eq!(painted.size, bounds.size);
    }
}
