use gpui::Context;
use settings::{Key, Read as _, Value};
use sidebarview::Sidebar;

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
            .showing("")
            .and_then(layout::Tab::focused)
            .and_then(|pane| layout.pane(pane));
        let now = focused.and_then(|pane| surface_of(pane.type_id.as_str()));
        let stored = self
            .settings_backend()
            .and_then(|backend| backend.get(&Key::new(SIDEBAR_FOLLOWS_FOCUS)));
        let moved = crossing(&mut self.focus_surface, now);
        if let Some(mode) = moved.filter(|_| follows(stored)) {
            self.sidebar
                .update(cx, |sidebar: &mut Sidebar, cx| sidebar.set_mode(mode, cx));
        }
    }
}

fn follows(stored: Option<Value>) -> bool {
    stored.and_then(|value| value.as_bool()).unwrap_or(false)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_stored_true_makes_the_sidebar_follow_focus() {
        let cases = [
            (Some(Value::Bool(true)), true),
            (Some(Value::Bool(false)), false),
            (Some(Value::Text("true".into())), false),
            (None, false),
        ];
        for (stored, expected) in cases {
            assert_eq!(follows(stored.clone()), expected, "{stored:?}");
        }
    }
}
