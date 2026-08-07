//! The window's root: the sidebar, the splitter, and the pane later slices
//! fill.
//!
//! The native half of `web/src/components/layout/ide-shell.tsx`'s
//! `ResizablePanelGroup` — a sidebar panel, a 1px separator, and a content
//! panel — reduced to what Slice 1a builds. The content panel is deliberately
//! empty: tabs and panes are slices 1b and 1c.

use crowbar_state::SidebarStore;
use crowbar_ui::Unanchored;
use crowbar_ui::gpui::{
    Context, Entity, IntoElement, ParentElement as _, Render, SharedString, Styled as _, Window,
    div, px,
};
use crowbar_ui::primitives::separator::{CallSite, Orientation, Separator};
use crowbar_ui::surfaces::rows::row_base;
use crowbar_ui::theme::Theme;

use super::Sidebar;

/// The shipping window's whole view.
pub struct Shell {
    /// The sidebar, which owns its own store handle.
    pub sidebar: Entity<Sidebar>,
    /// Item 0.4's daemon round trip, still shown so the transport does not
    /// quietly stop being exercised. It moves into the connection indicator
    /// once that surface exists.
    pub caption: SharedString,
    /// The store, read for the panel's width and open state.
    pub store: Entity<SidebarStore>,
}

impl Render for Shell {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::DARK;
        let panel = self.store.read(cx).panel();
        let open = panel.is_open();
        let width = panel.preferred_width();

        let mut row = div().flex().size_full();

        // Hidden is rendered as zero width rather than as an absent child:
        // `SidebarPeek` is a wrapper and not a branch in the reference, so
        // hiding the sidebar must not rebuild the subtree below it.
        if open {
            row = row
                .child(
                    div()
                        .w(px(width))
                        .flex_shrink_0()
                        .h_full()
                        .child(self.sidebar.clone()),
                )
                .child(
                    Separator {
                        orientation: Orientation::Vertical,
                        call_site: CallSite::None,
                    }
                    .render(&theme, &Unanchored),
                );
        }

        // The content pane is empty by design — tabs and panes are slices 1b
        // and 1c. It carries item 0.4's daemon round trip until the
        // connection indicator takes it over, so a build that has stopped
        // reaching the daemon still says so somewhere visible.
        row.child(
            div()
                .flex_1()
                .min_w(px(0.0))
                .h_full()
                .flex()
                .flex_col()
                .justify_end()
                .child(
                    // No padding literal: `Space` is a sealed newtype and the
                    // theme has no generic scale, only named tokens — which is
                    // the point. The sidebar's own row gutter is the nearest
                    // token that means anything here.
                    div()
                        .m(row_base::MARGIN_X)
                        .text_color(theme.muted_foreground)
                        .text_size(theme.ui_text_xs.value())
                        .font(crowbar_ui::ui_sans_font(&theme))
                        .child(self.caption.clone()),
                ),
        )
    }
}
