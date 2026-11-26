//! The frame: sidebar column, separator and content, plus the narrow overlay and the peek card.

pub(super) mod geometry;

use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, Context, Div, Entity, InteractiveElement, IntoElement, MouseButton, MouseDownEvent,
    ParentElement, Render, Stateful, StyleRefinement, Styled, Window, div, px,
};
use sidebar::{DockSide, Visibility};
use ui::{HoverFill, Motion, Portal, Surface, Theme};

use crate::sidebar::Sidebar;
use crate::sidebar::layout::geometry::Geometry;
use crate::sidebar::scale::{HANDLE_HIT, PEEK_INSET};

const FRAME_SHADOW: f32 = 0.2;

const SCRIM_ALPHA: f32 = 0.3;
const DRAWER_SHADOW: f32 = 0.1;
// PLATFORM: Chrome spreads develop's 1px line over two pixels at half alpha; GPUI snaps quads.
const HOVER_LINE_ALPHA: f32 = 0.3;
const LINE_SPAN: f32 = 2.0;

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
        let content = div().flex_1().min_w_0().h_full().child(
            self.content
                .clone()
                .cached(StyleRefinement::default().size_full()),
        );
        let side_column = div().h_full().flex_none().children(column);
        let handle = self.handle(&geometry, window, cx);
        let row = div()
            .flex()
            .size_full()
            .bg(palette.chrome_bg())
            .when(self.drag.lifted.is_some(), Styled::cursor_grabbing)
            .on_mouse_move(cx.listener(Self::track_pointer))
            .on_mouse_up(
                MouseButton::Left,
                cx.listener(|this, _, _, cx| this.release_pointer(cx)),
            );
        let row = match geometry.side {
            DockSide::Left => row.child(side_column).child(handle).child(content),
            DockSide::Right => row.child(content).child(handle).child(side_column),
        };
        if self.peek.is_moving(Motion::of(cx)) {
            window.request_animation_frame();
        }
        let sidebar = cx.entity();
        let float = self.float_kind(&geometry);
        let layers = self
            .floats(&geometry, cx)
            .then(|| floating_layers(sidebar, geometry).float(float));
        row.child(Self::exit_listener(cx)).children(layers)
    }
}

fn floating_layers(
    sidebar: Entity<Sidebar>,
    geometry: Geometry,
) -> Portal {
    Portal::cover("sidebar-layers", move |window, cx| {
        sidebar.update(cx, |this, cx| this.layers(&geometry, window, cx))
    })
}

impl Sidebar {
    fn float_kind(
        &self,
        geometry: &Geometry,
    ) -> ui::Float {
        let asks = self.tree.removals().confirmation().is_some()
            || self.tree.files().dialog().is_some()
            || self.question.is_some();
        if asks || geometry.is_drawer() {
            return ui::Float::Modal;
        }
        if self.line_target().is_some() {
            return ui::Float::Drag;
        }
        ui::Float::Popover
    }

    fn floats(
        &self,
        geometry: &Geometry,
        cx: &Context<'_, Self>,
    ) -> bool {
        geometry.is_drawer()
            || self.peek.is_shown(Motion::of(cx))
            || self.drawn_confirmation(cx).is_some()
            || self.tree.files().dialog().is_some()
            || self.question.is_some()
            || self.line_target().is_some()
            || self.menu.is_some()
    }

    fn layers(
        &mut self,
        geometry: &Geometry,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        div()
            .relative()
            .size_full()
            .children(self.floating(geometry, window, cx))
            .children(self.confirm_layer(cx))
            .children(self.file_dialog_layer(cx))
            .children(self.question_layer(cx))
            .children(self.drop_line_layer(window, cx))
            .children(self.menu_layer(cx))
    }

    fn handle(
        &self,
        geometry: &Geometry,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Stateful<Div> {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let line = || {
            div()
                .absolute()
                .top_0()
                .h_full()
                .w(px(LINE_SPAN))
                .left(px(HANDLE_HIT / 2.0))
        };
        let hover = palette.border().opacity(HOVER_LINE_ALPHA);
        let held = self.resizing.is_some().then(|| line().bg(hover));
        let hit = div()
            .track_focus(&self.handle_focus)
            .on_key_down(cx.listener(Self::press_separator))
            .on_mouse_down(MouseButton::Left, cx.listener(Self::press_handle))
            .absolute()
            .top_0()
            .h_full()
            .w(px(HANDLE_HIT))
            .left(px(-HANDLE_HIT / 2.0))
            .when(geometry.is_docked(), Styled::cursor_col_resize)
            .children(held);
        let hit =
            HoverFill::new("sidebar-separator", hover).apply_to_part(hit, line(), (window, cx));
        div()
            .id("sidebar-track")
            .relative()
            .flex_none()
            .h_full()
            .w(px(1.0))
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
        let layer = div().absolute().size_full();
        if geometry.is_drawer() {
            let drawer = self.drawer(geometry, window, cx);
            return Some(
                layer
                    .occlude()
                    .child(ui::Shaped::catch(Self::scrim(cx)))
                    .child(ui::Shaped::solid(drawer, px(0.0)))
                    .into_any_element(),
            );
        }
        let width = self
            .panel
            .preferred()
            .min(geometry.container - 2.0 * PEEK_INSET);
        let slide = self.peek.position(motion);
        let card = ui::Shaped::solid(
            self.card((geometry.side, width, slide), window, cx),
            ui::radius::XL,
        );
        let area = self.peek_area(card, window, cx);
        Some(layer.child(area).into_any_element())
    }

    fn drawer(
        &mut self,
        geometry: &Geometry,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let width = self.panel.preferred().min(geometry.container);
        div()
            .absolute()
            .top_0()
            .bottom_0()
            .w(px(width))
            .bg(palette.background())
            .shadow(ui::shadow::xl(DRAWER_SHADOW))
            .debug_selector(|| "sidebar-drawer".into())
            .when(geometry.side == DockSide::Left, Styled::left_0)
            .when(geometry.side == DockSide::Right, Styled::right_0)
            .occlude()
            .child(self.body(width, window, cx))
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
            .bg(palette.popover())
            .shadow(ui::shadow::lg(FRAME_SHADOW))
            .when(side == DockSide::Left, |d| d.left(px(inset)))
            .when(side == DockSide::Right, |d| d.right(px(inset)))
            .child(div().absolute().inset_0().bg(palette.muted()))
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
            self.changed(cx);
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

    #[gpui::test]
    fn a_narrow_window_lays_the_sidebar_over_its_whole_height_at_the_edge(cx: &mut TestAppContext) {
        let mut floating = crate::test_support::open_floating(cx, DemoShell::reference());
        floating
            .window
            .simulate_resize(gpui::size(px(600.0), px(800.0)));
        floating.window.run_until_parked();
        let drawer = floating
            .window
            .debug_bounds("sidebar-drawer")
            .unwrap_or_default();
        let placed = (
            f32::from(drawer.origin.x),
            f32::from(drawer.origin.y),
            f32::from(drawer.size.width),
            f32::from(drawer.size.height),
        );
        assert_eq!(placed, (0.0, 0.0, 294.0, 800.0));
    }
}
