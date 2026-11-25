use std::rc::Rc;

use gpui::{
    AnyElement, App, AppContext as _, Context, Entity, IntoElement, ParentElement as _, Pixels,
    Point, Render, Styled as _, Window, div,
};
use protocol::PaneId;
use ui::{DragGhost, Portal, layer};

use crate::pane_source::PaneSource;

const GHOST: &str = "grip-ghost";
const PILL_WIDTH: f32 = 160.0;

pub(crate) struct GripGhost {
    pane: PaneId,
    source: Rc<dyn PaneSource>,
    held: Point<Pixels>,
    pill: Entity<DragGhost>,
}

impl GripGhost {
    pub(crate) fn lift(
        (pane, source): (PaneId, Rc<dyn PaneSource>),
        held: Point<Pixels>,
        cx: &mut App,
    ) -> Entity<Self> {
        let label = source.label(&pane);
        let pill = cx.new(|_| DragGhost::new(label, PILL_WIDTH));
        cx.new(|_| Self {
            pane,
            source,
            held,
            pill,
        })
    }
}

impl Render for GripGhost {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        if self.source.preview(&self.pane, cx).is_none() {
            return self.pill.clone().into_any_element();
        }
        let (pane, source, held) = (self.pane.clone(), Rc::clone(&self.source), self.held);
        let build = move |_: &mut Window, cx: &mut App| {
            ui::Shaped::seen(centred(source.preview(&pane, cx), held), ui::radius::LG)
        };
        Portal::lifted(GHOST, build)
            .priority(layer::TOOLTIP)
            .into_any_element()
    }
}

fn centred(
    face: Option<AnyElement>,
    held: Point<Pixels>,
) -> gpui::Div {
    let centre = div()
        .size_0()
        .flex()
        .items_center()
        .justify_center()
        .child(div().flex_none().children(face));
    div().relative().left(held.x).top(held.y).child(centre)
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, MouseButton, Point, TestAppContext, point, px};
    use layout::Edge;

    use crate::hover_source::HoverSource;
    use crate::test_support::{
        FakeSource, Floating, PREVIEW, PREVIEW_HEIGHT, PREVIEW_WIDTH, layout_of,
        open_floating_with, pane_id,
    };

    const GRIP_CENTRE: (f32, f32) = (7.5 + 6.0, 5.0 + 6.0);

    fn handle_shown(
        source: FakeSource,
        cx: &mut TestAppContext,
    ) -> Floating {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let mut floating = open_floating_with(cx, &layout, source);
        let (view, window) = (floating.view.clone(), &mut floating.window);
        view.update(window, |view, cx| {
            view.pointer_entered((HoverSource::Cell, &pane_id("a")), cx);
        });
        floating
            .clock
            .set(floating.clock.get() + std::time::Duration::from_secs(1));
        view.update(window, |_, cx| cx.notify());
        window.run_until_parked();
        floating
    }

    fn drag_grip(
        floating: &mut Floating,
        to: Point<gpui::Pixels>,
    ) {
        let handle = floating
            .top()
            .debug_bounds("pane-handle")
            .expect("a handle");
        let grip = handle.origin + point(px(GRIP_CENTRE.0), px(GRIP_CENTRE.1));
        let (mut top, none) = (floating.top(), Modifiers::none());
        top.simulate_mouse_down(grip, MouseButton::Left, none);
        top.simulate_mouse_move(grip + point(px(20.0), px(0.0)), MouseButton::Left, none);
        top.simulate_mouse_move(to, MouseButton::Left, none);
        top.run_until_parked();
    }

    #[gpui::test]
    fn the_grip_lifts_the_pane_as_its_source_previews_it_held_by_its_centre(
        cx: &mut TestAppContext
    ) {
        let source = FakeSource::starting_at_first_tab().previewing(pane_id("a"));
        let mut floating = handle_shown(source, cx);
        let pointer = point(px(300.0), px(200.0));
        drag_grip(&mut floating, pointer);
        let preview = floating
            .top()
            .debug_bounds(PREVIEW)
            .expect("the preview is lifted");
        let half = point(px(PREVIEW_WIDTH / 2.0), px(PREVIEW_HEIGHT / 2.0));
        assert_eq!(preview.origin + half, pointer, "held by its centre");
        assert_eq!(floating.top().debug_bounds("drag-ghost"), None, "no pill");
    }

    #[gpui::test]
    fn a_pane_its_source_cannot_preview_lifts_as_the_labelled_pill(cx: &mut TestAppContext) {
        let mut floating = handle_shown(FakeSource::starting_at_first_tab(), cx);
        drag_grip(&mut floating, point(px(300.0), px(200.0)));
        assert!(
            floating.top().debug_bounds("drag-ghost").is_some(),
            "the pill"
        );
        assert_eq!(floating.top().debug_bounds(PREVIEW), None, "no preview");
    }
}
