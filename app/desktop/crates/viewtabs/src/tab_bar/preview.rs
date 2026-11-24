use gpui::{AnyElement, InteractiveElement as _, IntoElement as _, ParentElement as _, div};
use protocol::PaneId;

use crate::pill::{MemberView, ShellLook, TabShell};
use crate::strip_metrics::{SHELL_MARGIN, slot_width};
use crate::tab_bar::TabBar;

const PREVIEW: &str = "tab-preview";

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
        let preview = div().debug_selector(|| PREVIEW.into()).child(shell);
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
}

#[cfg(test)]
mod tests {
    use gpui::{
        Context, Entity, IntoElement, ParentElement as _, Render, Styled as _, TestAppContext,
        Window, div, px, size,
    };
    use protocol::PaneId;

    use crate::tab_bar::TabBar;

    use crate::strip_metrics::{SHELL_MARGIN, SLOT_MAX, TAB_HEIGHT};
    use crate::test_support::{open, view};

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

    #[gpui::test]
    fn a_pane_no_tab_lists_has_no_preview(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"])], 1);
        let unlisted = PaneId::new("z").expect("id");
        let preview = harness
            .bar
            .read_with(harness.cx, |bar, _| bar.preview(&unlisted).is_none());
        assert!(preview, "nothing to preview");
    }
}
