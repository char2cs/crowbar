use gpui::StatefulInteractiveElement;
use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, Context, InteractiveElement, IntoElement, ParentElement, Styled, Window, div, px,
};
use sidebar::DockSide;
use ui::Pose;

use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn peek_area(
        &self,
        card: ui::Shaped,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        if !self.peek.is_wanted() {
            return card.into_any_element();
        }
        let zone = self.peek_zone(window);
        let sidebar = cx.entity();
        let area = div()
            .id("sidebar-peek-zone")
            .absolute()
            .top_0()
            .bottom_0()
            .w(px(zone.keep_width()))
            .when(zone.side == DockSide::Left, Styled::left_0)
            .when(zone.side == DockSide::Right, Styled::right_0)
            .occlude()
            .on_hover(move |hovered, window, cx| {
                let pose = Pose::from(*hovered);
                sidebar.update(cx, |this, cx| this.follow_peek_hover(pose, window, cx));
            })
            .child(card);
        area.into_any_element()
    }

    // PLATFORM: linux a web view is its own X window, so the pointer leaves the window, not the area, when it moves onto one.
    fn follow_peek_hover(
        &mut self,
        pose: Pose,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) {
        let at = window.mouse_position();
        let kept = self
            .peek_zone(window)
            .keeps(f32::from(at.x), f32::from(at.y));
        if pose == Pose::Off && !kept {
            self.pointer_left_peek(cx);
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, point};

    use super::*;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn the_pointer_leaving_the_kept_area_dismisses_the_card(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update_in(cx, Sidebar::toggle);
        sidebar.update(cx, Sidebar::begin_peek);
        cx.run_until_parked();
        cx.simulate_mouse_move(point(px(100.0), px(300.0)), None, gpui::Modifiers::none());
        assert!(
            sidebar.read_with(cx, |this, _| this.peek.is_wanted()),
            "over the card"
        );
        cx.simulate_mouse_move(point(px(900.0), px(300.0)), None, gpui::Modifiers::none());
        assert!(
            !sidebar.read_with(cx, |this, _| this.peek.is_wanted()),
            "past it"
        );
    }
}
