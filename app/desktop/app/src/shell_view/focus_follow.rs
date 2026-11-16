use gpui::Context;
use settings_ui::{Key, SettingsBackend as _};
use sidebar::Sidebar;

use super::ShellView;
use crate::sidebar_surface::{crossing, surface_of};

pub(super) const SIDEBAR_FOLLOWS_FOCUS: &str = "sidebarFollowsFocus";

impl ShellView {
    pub(super) fn follow_focus(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let layout = self.state.layout();
        let focused = layout
            .active_tab()
            .and_then(model::Tab::focused)
            .and_then(|pane| layout.pane(pane));
        let now = focused.and_then(|pane| surface_of(pane.type_id.as_str()));
        let moved = crossing(&mut self.focus_surface, now);
        let follows = self
            .settings_backend()
            .and_then(|backend| backend.get(&Key::new(SIDEBAR_FOLLOWS_FOCUS)))
            .and_then(|value| value.as_bool())
            .unwrap_or(false);
        if let Some(mode) = moved.filter(|_| follows) {
            self.sidebar
                .update(cx, |sidebar: &mut Sidebar, cx| sidebar.set_mode(mode, cx));
        }
    }
}
