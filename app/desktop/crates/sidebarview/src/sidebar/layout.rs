//! The frame: sidebar column, separator and content, plus the narrow overlay and the peek card.

use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, Context, Div, InteractiveElement, IntoElement, MouseButton, MouseDownEvent,
    ParentElement, Render, Stateful, Styled, Window, div, px,
};
use sidebar::{DockSide, Visibility};
use ui::{Motion, Surface, Theme};

use crate::sidebar::Sidebar;
use crate::sidebar::geometry::Geometry;
use crate::sidebar::scale::{HANDLE_HIT, PEEK_INSET};

const FRAME_SHADOW: f32 = 0.2;

const SCRIM_ALPHA: f32 = 0.4;

impl Render for Sidebar {
    fn render(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let container = Self::container_width(window);
        let geometry = Geometry::new(container, self.live_width(container), self.panel);
        let column = geometry
            .is_docked()
            .then(|| self.body(geometry.width, window, cx));
        let content = div()
            .flex_1()
            .min_w_0()
            .h_full()
            .child(self.content.clone());
        let side_column = div().h_full().flex_none().children(column);
        let handle = self.handle(&geometry, cx);
        let row = div()
            .flex()
            .size_full()
            .bg(palette.chrome_bg())
            .when(self.drag.lifted.is_some(), Styled::cursor_grabbing)
            .on_mouse_move(cx.listener(|this, event, window, cx| {
                this.move_resize(event, cx);
                this.track_peek(event.position, window, cx);
            }))
            .on_mouse_up(
                MouseButton::Left,
                cx.listener(|this, _, _, cx| this.release_pointer(cx)),
            );
        let row = match geometry.side {
            DockSide::Left => row.child(side_column).child(handle).child(content),
            DockSide::Right => row.child(content).child(handle).child(side_column),
        };
        row.child(Self::exit_listener(cx))
            .children(self.floating(&geometry, window, cx))
            .children(self.confirm_layer(cx))
            .children(self.file_dialog_layer(cx))
            .children(self.question_layer(cx))
            .children(self.drop_line_layer(window, cx))
            .children(self.menu_layer(window, cx))
    }
}

impl Sidebar {
    fn handle(
        &self,
        geometry: &Geometry,
        cx: &mut Context<'_, Self>,
    ) -> Stateful<Div> {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let line = div()
            .h_full()
            .w(px(1.0))
            .bg(palette.border())
            .when(self.resizing.is_some(), |d| d.bg(palette.ring()));
        let hit = div()
            .id("sidebar-separator")
            .track_focus(&self.handle_focus)
            .on_key_down(cx.listener(Self::press_separator))
            .on_mouse_down(MouseButton::Left, cx.listener(Self::press_handle))
            .absolute()
            .top_0()
            .h_full()
            .w(px(HANDLE_HIT))
            .when(geometry.side == DockSide::Left, |d| {
                d.left(px(-HANDLE_HIT / 2.0))
            })
            .when(geometry.side == DockSide::Right, |d| {
                d.right(px(-HANDLE_HIT / 2.0))
            })
            .when(geometry.is_docked(), Styled::cursor_col_resize);
        div()
            .id("sidebar-track")
            .relative()
            .flex_none()
            .h_full()
            .w(px(1.0))
            .child(line)
            .child(hit)
    }

    fn press_handle(
        &mut self,
        event: &MouseDownEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if event.click_count >= 2 {
            self.toggle_collapsed(cx);
            return;
        }
        self.begin_resize(event.position.x, window, cx);
    }

    fn slide(
        &self,
        geometry: &Geometry,
        motion: Motion,
    ) -> f32 {
        if geometry.is_drawer() {
            return 1.0;
        }
        self.peek.position(motion)
    }

    fn floating(
        &mut self,
        geometry: &Geometry,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let motion = Motion::of(cx);
        if !geometry.is_drawer() && !self.peek.is_shown(motion) {
            return None;
        }
        if self.peek.is_moving(motion) {
            window.request_animation_frame();
        }
        let width = self
            .panel
            .preferred()
            .min(geometry.container - 2.0 * PEEK_INSET);
        let slide = self.slide(geometry, motion);
        let card = self.card((geometry.side, width, slide), window, cx);
        let layer = div().absolute().size_full();
        if geometry.is_drawer() {
            return Some(
                layer
                    .occlude()
                    .child(Self::scrim(cx))
                    .child(card)
                    .into_any_element(),
            );
        }
        Some(layer.child(card).into_any_element())
    }

    fn card(
        &mut self,
        placement: (DockSide, f32, f32),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let (side, width, slide) = placement;
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let inset = PEEK_INSET - (width + PEEK_INSET) * (1.0 - slide);
        div()
            .absolute()
            .top(px(PEEK_INSET))
            .bottom(px(PEEK_INSET))
            .w(px(width))
            .rounded(ui::radius::XL)
            .overflow_hidden()
            .border_1()
            .border_color(palette.border())
            .bg(palette.chrome_bg())
            .shadow(ui::shadow::lg(FRAME_SHADOW))
            .when(side == DockSide::Left, |d| d.left(px(inset)))
            .when(side == DockSide::Right, |d| d.right(px(inset)))
            .occlude()
            .child(self.body(width, window, cx))
    }

    fn scrim(cx: &mut Context<'_, Self>) -> Stateful<Div> {
        div()
            .id("sidebar-scrim")
            .absolute()
            .size_full()
            .bg(gpui::hsla(0.0, 0.0, 0.0, SCRIM_ALPHA))
            .on_mouse_down(
                MouseButton::Left,
                cx.listener(|this, _, _, cx| this.dismiss_floating(cx)),
            )
    }

    fn dismiss_floating(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.peek.slide_to(0.0, Motion::of(cx));
        if self.panel.visibility() == Visibility::Open {
            self.panel.toggle();
            Self::changed(cx);
        }
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::Visibility;

    use crate::test_support::prelude::*;

    use sidebar::DockSide;

    #[gpui::test]
    fn an_open_sidebar_in_a_wide_window_renders_docked(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        cx.run_until_parked();
        let width = sidebar.read_with(cx, |this, _| this.live_width(1200.0));
        assert_eq!(width, 294.0);
    }

    #[gpui::test]
    fn the_scrim_closes_a_drawer(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, Sidebar::dismiss_floating);
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.preferences().visibility),
            Visibility::Closed
        );
    }

    #[gpui::test]
    fn a_right_dock_renders_without_panicking(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        sidebar.update(cx, |this, cx| this.dock_to(DockSide::Right, cx));
        cx.run_until_parked();
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.preferences().side),
            DockSide::Right
        );
    }

    #[gpui::test]
    fn a_single_click_on_the_separator_starts_a_drag(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.begin_resize(px(294.0), window, cx);
        });
        assert!(sidebar.read_with(cx, |this, _| this.resizing.is_some()));
    }
}
