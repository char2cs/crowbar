use gpui::{App, Context, Window};

use crate::shell::shell_view::ShellView;

impl ShellView {
    // PLATFORM: linux X focus reports own the window once they arrive; GPUI's activation is then an echo of web-view focus.
    pub(super) fn on_gpui_activation(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if ui::WindowActivity::is_reported(window, cx) {
            return;
        }
        self.on_window_activation(window, cx);
    }

    pub(super) fn on_window_activation(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if !ui::WindowActivity::of(window, cx) {
            self.sidebar
                .update(cx, |sidebar, cx| sidebar.cancel_drag(window, cx));
        }
        self.notices
            .update(cx, |notices, cx| notices.follow_window(window, cx));
        self.return_keyboard(window, cx);
    }

    fn return_keyboard(
        &self,
        window: &Window,
        cx: &App,
    ) {
        if !ui::WindowActivity::of(window, cx) {
            return;
        }
        self.state.apply_keyboard();
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::shell::shell_view::tests::{
        open_started_shell, own_keyboard_by_page, page_focuses, register_chat,
    };

    #[gpui::test]
    fn gpui_s_own_deactivation_leaves_the_keyboard_where_the_platform_reports_it(
        cx: &mut TestAppContext
    ) {
        let mut opened = open_started_shell(cx);
        register_chat(&mut opened);
        own_keyboard_by_page(&mut opened);
        opened.window.update(|window, cx| {
            ui::WindowActivity::gained(window.window_handle().window_id(), cx);
        });
        opened.window.run_until_parked();
        let before = page_focuses(&opened.mock);
        opened.window.deactivate_window();
        opened.window.run_until_parked();
        assert_eq!(
            page_focuses(&opened.mock),
            before,
            "another window took the keyboard; the page must not grab it back"
        );
    }
}
