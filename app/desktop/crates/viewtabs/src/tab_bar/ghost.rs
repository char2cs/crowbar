use gpui::{
    AnyElement, App, InteractiveElement, IntoElement, ParentElement, Styled, Window, div, px,
};
use ui::{Portal, layer};

use crate::gesture::band;
use crate::pill::{MemberView, ShellLook, TabShell};
use crate::strip_metrics::{ROW_HEIGHT, SHELL_MARGIN, TAB_HEIGHT};
use crate::tab_bar::TabBar;

const GHOST: &str = "tab-ghost";

impl TabBar {
    pub(crate) fn ghost(&self) -> Option<AnyElement> {
        let drag = self.drag.as_ref()?;
        let tab = self
            .tabs
            .iter()
            .find(|tab| Some(tab.id) == drag.engine.tab())?;
        let row_top = (ROW_HEIGHT - TAB_HEIGHT) / 2.0;
        let pointer_y = f32::from(drag.pointer.y - self.row.origin.y);
        let grab_y = drag.grab_y + row_top;
        let top = band::ghost_y(pointer_y, row_top, grab_y);
        let pull = band::pull(pointer_y, row_top, grab_y);
        let x = drag.engine.eased_x(self.content_x(drag.pointer), pull);
        let left = x
            + f32::from(self.scroll.offset().x)
            + f32::from(self.strip_left() - self.row.origin.x);
        let width = self.slot_spans().first().map_or(0.0, |span| span.width) - 2.0 * SHELL_MARGIN;
        let members = tab.members.clone();
        let place = (left + SHELL_MARGIN, top);
        let build = move |_: &mut Window, _: &mut App| lifted(members.clone(), width, place);
        let ghost = Portal::here(GHOST, build).priority(layer::TOOLTIP);
        Some(div().absolute().inset_0().child(ghost).into_any_element())
    }
}

fn lifted(
    members: Vec<MemberView>,
    width: f32,
    (left, top): (f32, f32),
) -> gpui::Div {
    let shell = TabShell {
        id: GHOST.into(),
        members,
        look: ShellLook::Lifted,
        width,
        on_close: None,
    };
    div()
        .debug_selector(|| GHOST.into())
        .absolute()
        .left(px(left))
        .top(px(top))
        .child(shell)
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, MouseButton, TestAppContext, point, px};

    use crate::test_support::{open_floating, view};

    #[gpui::test]
    fn a_tab_dragged_out_of_the_strip_floats_above_the_pane_web_views(cx: &mut TestAppContext) {
        let tabs = vec![view(1, &["a"]), view(2, &["b"])];
        let mut floating = open_floating(cx, tabs, 1);
        let window = &mut floating.window;
        let none = Modifiers::none();
        window.simulate_mouse_down(point(px(108.0), px(21.0)), MouseButton::Left, none);
        window.simulate_mouse_move(point(px(130.0), px(21.0)), MouseButton::Left, none);
        window.simulate_mouse_move(point(px(400.0), px(300.0)), MouseButton::Left, none);
        window.run_until_parked();
        assert_eq!(
            window.debug_bounds("tab-ghost"),
            None,
            "nothing in the window"
        );
        let ghost = floating.top.debug_bounds("tab-ghost");
        assert!(ghost.is_some(), "the ghost is in the top layer");
    }
}
