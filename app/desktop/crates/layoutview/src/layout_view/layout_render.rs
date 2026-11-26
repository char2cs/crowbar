use std::time::Instant;

use gpui::{
    AnyElement, App, Context, Hsla, InteractiveElement, IntoElement, MouseButton, MouseDownEvent,
    ParentElement, Render, Styled, Window, canvas, div, px,
};
use layout::{Axis, DragSubject, Sash as ModelSash};
use protocol::PaneId;
use ui::{DraggedRow, Motion, Portal, Sash, SashDrag, Surface, Theme, Wordmark};

use crate::geometry::pane_frame::PaneFrame;
use crate::layout_view::LayoutView;
use crate::layout_view::hover_source::HoverSource;
use crate::layout_view::pane::pane_cell::pane_box;
use crate::layout_view::pane::pane_handle::PaneHandle;
use ideshell::STRIP_HEIGHT;

const LOGO_SHARE: f32 = 0.14;
const LOGO_MIN_WIDTH: f32 = 96.0;
const LOGO_MAX_WIDTH: f32 = 148.0;
const PREVIEW_BORDER: f32 = 2.0;
const PREVIEW_FILL: f32 = 0.14;
const PREVIEW_RING: f32 = 0.25;

impl Render for LayoutView {
    fn render(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let (now, motion) = ((self.clock)(), Motion::of(cx));
        let busy = self.rings.is_running(now, motion)
            || self.handles.is_running(now, motion)
            || self.drop.is_running(now, motion);
        if busy {
            window.request_animation_frame();
        }
        let view = cx.entity();
        div()
            .id("layout-view")
            .relative()
            .size_full()
            .child(
                canvas(
                    move |bounds, _, cx: &mut App| {
                        view.update(cx, |this, cx| this.measured(bounds, cx));
                    },
                    |_, (), _, _| {},
                )
                .absolute()
                .size_full(),
            )
            .children(self.logo(cx))
            .children(self.pane_cells(window, cx))
            .children(self.sashes(cx))
            .children(self.drop_layer(cx))
            .on_drag_move::<DragSubject>(cx.listener(Self::drag_over))
            .on_drag_move::<DraggedRow>(cx.listener(Self::row_over))
            .on_drag_move::<SashDrag>(cx.listener(Self::sash_moved))
            .on_drop::<DragSubject>(cx.listener(Self::dropped))
            .on_mouse_up(MouseButton::Left, cx.listener(Self::row_released))
            .on_mouse_up_out(MouseButton::Left, cx.listener(Self::released))
    }
}

impl LayoutView {
    pub(crate) fn shows_logo(&self) -> bool {
        self.tab.is_none()
    }

    fn logo(
        &self,
        cx: &App,
    ) -> Option<AnyElement> {
        if !self.shows_logo() {
            return None;
        }
        let smaller_side = f32::from(self.bounds.size.width.min(self.bounds.size.height));
        let width = (smaller_side * LOGO_SHARE).clamp(LOGO_MIN_WIDTH, LOGO_MAX_WIDTH);
        let local = self.local();
        let frame = PaneFrame::new(local, local, self.chrome);
        let palette = Theme::of(cx).palette(Surface::Window);
        let stage = pane_box(&frame, local, palette)
            .bg(palette.pane_background())
            .pt(px(STRIP_HEIGHT))
            .flex()
            .items_center()
            .justify_center()
            .child(Wordmark::new(px(width)));
        Some(stage.into_any_element())
    }

    pub(crate) fn handle_for(
        &self,
        pane: &PaneId,
        (now, motion): (Instant, Motion),
        cx: &mut Context<'_, Self>,
    ) -> Option<PaneHandle> {
        let opacity = self.handles.opacity(pane, now, motion);
        let crowded = self.tab.as_ref().is_some_and(|tab| tab.group_count() > 1);
        if !crowded || opacity <= 0.0 {
            return None;
        }
        Some(PaneHandle {
            pane: pane.clone(),
            source: std::rc::Rc::clone(&self.source),
            opacity,
            on_detach: Self::detach_action(pane, cx),
            on_close: Self::close_action(pane, cx),
            on_hover: [
                Self::leave_action((HoverSource::Handle, pane), cx),
                Self::enter_action((HoverSource::Handle, pane), cx),
            ],
        })
    }

    fn sashes(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Vec<AnyElement> {
        let Some(tab) = &self.tab else {
            return Vec::new();
        };
        tab.sashes(self.local())
            .into_iter()
            .enumerate()
            .map(|(index, sash)| Self::sash_element(index, &sash, cx))
            .collect()
    }

    fn sash_element(
        index: usize,
        sash: &ModelSash,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let rect = sash.rect;
        let axis = ui_axis(sash.axis);
        let grabbed = sash.clone();
        div()
            .id(("sash", index))
            .absolute()
            .left(px(rect.x))
            .top(px(rect.y))
            .w(px(rect.width))
            .h(px(rect.height))
            .on_mouse_down(
                MouseButton::Left,
                cx.listener(move |this, event: &MouseDownEvent, _, cx| {
                    this.grab_sash(&grabbed, event, cx);
                }),
            )
            .child(Sash::new(("sash-bar", index), axis).on(Surface::Window))
            .into_any_element()
    }

    fn drop_layer(
        &self,
        cx: &App,
    ) -> Option<AnyElement> {
        let rect = self.drop.preview((self.clock)(), Motion::of(cx))?;
        let accent = Theme::of(cx).palette(Surface::Window).secondary();
        let preview = Portal::here("drop-preview", move |_, _| {
            ui::Shaped::seen(drop_preview(rect, accent), ui::radius::LG)
        })
        .float(ui::Float::Drag);
        Some(div().absolute().inset_0().child(preview).into_any_element())
    }
}

fn ui_axis(axis: Axis) -> ui::Axis {
    match axis {
        Axis::Horizontal => ui::Axis::Horizontal,
        Axis::Vertical => ui::Axis::Vertical,
    }
}

fn drop_preview(
    rect: Rect,
    accent: Hsla,
) -> gpui::Div {
    div()
        .absolute()
        .left(px(rect.x))
        .top(px(rect.y))
        .w(px(rect.width))
        .h(px(rect.height))
        .child(tinted(accent))
}

fn tinted(accent: Hsla) -> gpui::Div {
    div()
        .debug_selector(|| "drop-preview".into())
        .absolute()
        .size_full()
        .rounded(ui::radius::LG)
        .border(px(PREVIEW_BORDER))
        .border_color(accent)
        .bg(accent.opacity(PREVIEW_FILL))
        .shadow(vec![gpui::BoxShadow {
            color: accent.opacity(PREVIEW_RING),
            offset: gpui::point(px(0.0), px(0.0)),
            blur_radius: px(0.0),
            spread_radius: px(1.0),
            inset: false,
        }])
}

#[cfg(test)]
mod tests {
    use gpui::{Bounds, TestAppContext, point, red, size};
    use layout::Rect;

    use super::*;

    struct Shown;

    impl Render for Shown {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let rect = Rect {
                x: 10.0,
                y: 20.0,
                width: 60.0,
                height: 30.0,
            };
            div()
                .relative()
                .size_full()
                .child(drop_preview(rect, red()))
        }
    }

    #[gpui::test]
    fn the_preview_tints_exactly_the_rectangle_it_is_given(cx: &mut TestAppContext) {
        let (_, window) = cx.add_window_view(|_, _| Shown);
        window.run_until_parked();
        let expected = Bounds::new(point(px(10.0), px(20.0)), size(px(60.0), px(30.0)));
        assert_eq!(window.debug_bounds("drop-preview"), Some(expected));
    }
}
