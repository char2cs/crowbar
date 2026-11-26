//! The lifted tab: the ghost of a dragged tab and the preview of a pane joining the strip.

use gpui::{
    AnyElement, App, InteractiveElement as _, IntoElement as _, ParentElement as _, Styled as _,
    Window, div, px,
};
use protocol::PaneId;
use ui::{Portal, layer};

use crate::pill::{MemberView, ShellLook, TabShell};
use crate::strip_metrics::{ROW_HEIGHT, SHELL_MARGIN, TAB_HEIGHT, slot_width};
use crate::tab_bar::TabBar;

const PREVIEW: &str = "tab-preview";
const GHOST: &str = "tab-ghost";
const BAND_MARGIN: f32 = 32.0;

impl TabBar {
    /// A pane's chat as the view tab it would become, lifted, at the width it takes on joining
    /// the strip; `None` for a pane no tab of the strip lists.
    #[must_use]
    pub fn preview(
        &self,
        pane: &PaneId,
    ) -> Option<AnyElement> {
        let member = self.member(pane)?.clone();
        let width = slot_width(self.tabs.len() + 1, self.strip_width()) - 2.0 * SHELL_MARGIN;
        let shell = TabShell {
            id: PREVIEW.into(),
            members: vec![member],
            look: ShellLook::Lifted,
            width,
            on_close: None,
        };
        let (dx, dy) = self.held_offset().unwrap_or_default();
        let preview = div()
            .debug_selector(|| PREVIEW.into())
            .relative()
            .left(px(dx))
            .top(px(dy))
            .child(shell);
        Some(preview.into_any_element())
    }

    pub(crate) fn member(
        &self,
        pane: &PaneId,
    ) -> Option<&MemberView> {
        self.tabs
            .iter()
            .flat_map(|tab| &tab.members)
            .find(|member| &member.pane == pane)
    }

    pub(crate) fn ghost(&self) -> Option<AnyElement> {
        let drag = self.drag.as_ref()?;
        let tab = self
            .tabs
            .iter()
            .find(|tab| Some(tab.id) == drag.engine.tab())?;
        let row_top = (ROW_HEIGHT - TAB_HEIGHT) / 2.0;
        let pointer_y = f32::from(drag.pointer.y - self.row.origin.y);
        let grab_y = drag.grab_y + row_top;
        let top = ghost_y(pointer_y, row_top, grab_y);
        let pull = pull(pointer_y, row_top, grab_y);
        let x = drag.engine.eased_x(self.content_x(drag.pointer), pull);
        let left = x
            + f32::from(self.scroll.offset().x)
            + f32::from(self.strip_left() - self.row.origin.x);
        let width = self.slot_spans().first().map_or(0.0, |span| span.width) - 2.0 * SHELL_MARGIN;
        let members = tab.members.clone();
        let place = (left + SHELL_MARGIN, top);
        let build = move |_: &mut Window, _: &mut App| {
            ui::Shaped::seen(lifted(members.clone(), width, place), ui::radius::LG)
        };
        let ghost = Portal::here(GHOST, build)
            .priority(layer::TOOLTIP)
            .float(ui::Float::Drag);
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

#[must_use]
fn pull(
    pointer_y: f32,
    row_top: f32,
    grab_y: f32,
) -> f32 {
    let delta = pointer_y - grab_y - row_top;
    (delta.abs() / BAND_MARGIN).clamp(0.0, 1.0)
}

#[must_use]
fn ghost_y(
    pointer_y: f32,
    row_top: f32,
    grab_y: f32,
) -> f32 {
    let delta = pointer_y - grab_y - row_top;
    row_top + delta * pull(pointer_y, row_top, grab_y)
}

#[cfg(test)]
mod tests {
    use gpui::{
        Context, Entity, IntoElement, Modifiers, MouseButton, ParentElement as _, Render,
        Styled as _, TestAppContext, Window, div, point, px, size,
    };
    use protocol::PaneId;

    use crate::tab_bar::TabBar;

    use crate::strip_metrics::{SHELL_MARGIN, SLOT_MAX, TAB_HEIGHT};
    use crate::test_support::{open, open_floating, view};

    struct Lifted(Entity<TabBar>);

    impl Render for Lifted {
        fn render(
            &mut self,
            _window: &mut Window,
            cx: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let preview = self.0.read(cx).preview(&PaneId::new("b").expect("id"));
            div().flex().items_start().children(preview)
        }
    }

    #[gpui::test]
    fn a_member_previews_as_a_lifted_tab_as_wide_as_a_tab_joining_the_strip(
        cx: &mut TestAppContext
    ) {
        let harness = open(cx, vec![view(1, &["a"]), view(2, &["b"])], 1);
        let (bar, cx) = (harness.bar, harness.cx);
        cx.simulate_resize(size(px(616.0), px(400.0)));
        cx.run_until_parked();
        let joined = bar.read_with(cx, |bar, _| bar.strip_width() / 3.0);
        assert!(joined < SLOT_MAX, "the strip is narrow enough to share");
        let (_, lifted) = cx.add_window_view(|_, _| Lifted(bar));
        lifted.run_until_parked();
        let bounds = lifted
            .debug_bounds("tab-preview")
            .expect("a preview is drawn");
        assert_eq!(
            bounds.size.width,
            px(joined - 2.0 * SHELL_MARGIN),
            "joined width"
        );
        assert_eq!(bounds.size.height, px(TAB_HEIGHT), "a tab's height");
    }

    fn held_preview_origin(
        at: gpui::Point<gpui::Pixels>,
        cx: &mut TestAppContext,
    ) -> gpui::Point<gpui::Pixels> {
        let harness = open(cx, vec![view(1, &["a"]), view(2, &["a2", "b"])], 1);
        let (bar, cx) = (harness.bar, harness.cx);
        cx.simulate_resize(size(px(616.0), px(400.0)));
        cx.run_until_parked();
        let subject = layout::DragSubject::Pane(PaneId::new("b").expect("id"));
        bar.update(cx, |bar, cx| bar.hover_at((at, Some(&subject)), cx));
        let (_, lifted) = cx.add_window_view(|_, _| Lifted(bar));
        lifted.run_until_parked();
        lifted
            .debug_bounds("tab-preview")
            .expect("a preview is drawn")
            .origin
    }

    #[gpui::test]
    fn a_member_held_over_the_strip_is_pinned_to_the_tab_row(cx: &mut TestAppContext) {
        let low_in_the_row = point(px(8.0 + 250.0), px(35.0));
        let origin = held_preview_origin(low_in_the_row, cx);
        assert_eq!(origin, point(px(0.0), px(5.0 - 19.0)), "raised");
    }

    #[gpui::test]
    fn a_pane_no_tab_lists_has_no_preview(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"])], 1);
        let unlisted = PaneId::new("z").expect("id");
        let preview = harness
            .bar
            .read_with(harness.cx, |bar, _| bar.preview(&unlisted).is_none());
        assert!(preview, "nothing to preview");
    }

    #[gpui::test]
    fn a_tab_dragged_out_of_the_strip_is_drawn_in_the_window_and_hides_the_web_views(
        cx: &mut TestAppContext
    ) {
        let tabs = vec![view(1, &["a"]), view(2, &["b"])];
        let mut floating = open_floating(cx, tabs, 1);
        let window = &mut floating.window;
        let none = Modifiers::none();
        window.simulate_mouse_down(point(px(108.0), px(21.0)), MouseButton::Left, none);
        window.simulate_mouse_move(point(px(130.0), px(21.0)), MouseButton::Left, none);
        window.simulate_mouse_move(point(px(400.0), px(300.0)), MouseButton::Left, none);
        window.run_until_parked();
        assert!(window.debug_bounds("tab-ghost").is_some(), "in the window");
        assert!(ui::testing::covered(window).0, "every web view is hidden");
    }

    const BAND_CASES: &[(f32, f32, &str)] = &[
        (26.0, 5.0, "level with the row"),
        (42.0, 13.0, "halfway eased"),
        (226.0, 205.0, "free beyond the margin"),
    ];

    #[test]
    fn the_ghost_is_pinned_to_the_row_near_it_and_free_far_from_it() {
        for (pointer_y, expected, name) in BAND_CASES {
            assert_eq!(super::ghost_y(*pointer_y, 5.0, 21.0), *expected, "{name}");
        }
    }
}
