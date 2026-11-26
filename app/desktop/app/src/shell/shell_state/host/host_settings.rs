use crate::shell::inbox::logged::emit;
use crate::shell::shell_state::ShellState;

mod theme_event;
use crate::panes::content::pane_theme;
use theme_event::ThemeEvent;

impl ShellState {
    pub(crate) fn set_keybindings(
        &mut self,
        bindings: Vec<protocol::KeyBinding>,
    ) {
        if let Err(error) = self.host.set_keybindings(bindings) {
            tracing::warn!(%error, "pushing the keybindings to the panes failed");
        }
    }

    pub(crate) fn set_theme(
        &mut self,
        theme: ui::Theme,
    ) {
        let tokens = match pane_theme::tokens_of(theme) {
            Ok(tokens) => tokens,
            Err(error) => return emit(&ThemeEvent::Undescribed { error: &error }),
        };
        if let Err(error) = self.host.set_theme(tokens) {
            emit(&ThemeEvent::Unsent { error: &error });
        }
        let surface = pane_theme::surface_color(theme);
        if let Err(error) = self.host.set_background(surface) {
            emit(&ThemeEvent::Uncoloured { error: &error });
        }
        let scheme = pane_theme::color_scheme(theme);
        if let Err(error) = self.host.desktop().draw_widgets_in(scheme) {
            emit(&ThemeEvent::Undrawn { error: &error });
        }
    }

    pub(crate) fn set_window_width(
        &mut self,
        width: f32,
    ) {
        let cleared = ui::Breakpoint::cleared_by(width)
            .into_iter()
            .map(wire_breakpoint)
            .collect();
        if let Err(error) = self.host.set_breakpoints(cleared) {
            tracing::warn!(%error, "sending the window's breakpoints to panes failed");
        }
    }

    pub(crate) fn set_pane_settings(
        &mut self,
        values: protocol::SettingValues,
    ) {
        if let Err(error) = self.host.set_settings(values) {
            tracing::warn!(%error, "sending the settings to panes failed");
        }
    }
}

fn wire_breakpoint(point: ui::Breakpoint) -> protocol::Breakpoint {
    match point {
        ui::Breakpoint::Sm => protocol::Breakpoint::Sm,
        ui::Breakpoint::Md => protocol::Breakpoint::Md,
        ui::Breakpoint::Lg => protocol::Breakpoint::Lg,
        ui::Breakpoint::Xl => protocol::Breakpoint::Xl,
    }
}

#[cfg(test)]
mod tests {
    use crate::test_support::{evals_with, register_chat, shell_rig, synced_chat};

    #[test]
    fn a_mounted_pane_learns_the_breakpoints_the_window_width_clears() {
        let width_cases = [
            (1100.0, r#"[\"sm\",\"md\",\"lg\"]"#),
            (4000.0, r#"[\"sm\",\"md\",\"lg\",\"xl\"]"#),
        ];
        for (width, cleared) in width_cases {
            let mut rig = shell_rig();
            let pane = synced_chat(&mut rig);
            register_chat(&mut rig, pane);
            rig.shell.set_window_width(width);
            let frame = format!(r#"{{\"sdk\":1,\"kind\":\"breakpoints\",\"cleared\":{cleared}}}"#);
            assert!(
                evals_with(&rig, &frame) > 0,
                "{width}: {:?}",
                rig.mock.calls()
            );
        }
    }

    #[test]
    fn a_theme_change_recolours_the_live_pane_views() {
        let mut rig = shell_rig();
        synced_chat(&mut rig);
        let theme = ui::Theme::default();
        rig.shell.set_theme(theme);
        let painted: Vec<_> = rig
            .mock
            .calls()
            .into_iter()
            .filter_map(|call| match call {
                webview::Call::SetBackground { color, .. } => Some(color),
                _ => None,
            })
            .collect();
        assert_eq!(
            painted,
            vec![crate::panes::content::pane_theme::surface_color(theme)]
        );
    }
}
