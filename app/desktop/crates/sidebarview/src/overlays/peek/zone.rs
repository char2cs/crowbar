//! The area that keeps the peek card open. It floats with the card in the top layer, above the
//! page, so it hears the pointer leave where the owner window no longer sees it.

use gpui::StatefulInteractiveElement;
use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, Context, Div, InteractiveElement, IntoElement, ParentElement, Styled, Window, div,
    px,
};
use sidebar::DockSide;
use ui::Pose;

use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn peek_area(
        &self,
        card: Div,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        if !self.peek.is_wanted() {
            return card.into_any_element();
        }
        let zone = self.peek_zone(window);
        let sidebar = cx.entity();
        div()
            .id("sidebar-peek-zone")
            .absolute()
            .top_0()
            .bottom_0()
            .w(px(zone.keep_width()))
            .when(zone.side == DockSide::Left, Styled::left_0)
            .when(zone.side == DockSide::Right, Styled::right_0)
            .occlude()
            .on_hover(move |hovered, _, cx| {
                let pose = Pose::from(*hovered);
                sidebar.update(cx, |this, cx| this.follow_peek_hover(pose, cx));
            })
            .child(card)
            .into_any_element()
    }

    fn follow_peek_hover(
        &mut self,
        pose: Pose,
        cx: &mut Context<'_, Self>,
    ) {
        if pose == Pose::Off {
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
