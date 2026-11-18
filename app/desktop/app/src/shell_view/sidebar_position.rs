use gpui::Context;
use settings::{Key, SettingsRead as _, Value};
use sidebar::DockSide;

use super::ShellView;

pub(super) const SIDEBAR_POSITION: &str = "sidebarPosition";

fn dock_of(stored: Option<&Value>) -> DockSide {
    match stored.and_then(Value::as_text) {
        Some("right") => DockSide::Right,
        _ => DockSide::Left,
    }
}

impl ShellView {
    pub(super) fn apply_sidebar_position(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(backend) = self.settings_backend() else {
            return;
        };
        let side = dock_of(backend.get(&Key::new(SIDEBAR_POSITION)).as_ref());
        self.sidebar
            .update(cx, |sidebar, cx| sidebar.dock_to(side, cx));
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_right_docks_the_sidebar_to_the_right() {
        let right = Value::Text("right".into());
        let left = Value::Text("left".into());
        let odd = Value::Text("middle".into());
        let cases = [
            (Some(&right), DockSide::Right),
            (Some(&left), DockSide::Left),
            (Some(&odd), DockSide::Left),
            (None, DockSide::Left),
        ];
        for (stored, expected) in cases {
            assert_eq!(dock_of(stored), expected, "{stored:?}");
        }
    }
}
