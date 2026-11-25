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
