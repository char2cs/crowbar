//! The shell's hooks into the desktop: its own menus, file choosers and message dialogs, which GTK
//! shows on the web views' platform thread in place of the drawn ones, and the float windows' placer.

mod alert;
mod file_chooser;
pub(crate) mod float;
mod menu;

use gpui::App;

pub(crate) fn install(
    host: &panehost::PaneHost,
    cx: &mut App,
) {
    let desktop = host.desktop();
    file_chooser::install(desktop.clone(), cx);
    menu::install(desktop.clone(), cx);
    alert::install(desktop, cx);
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::TestAppContext;
    use webview::MockPlatform;

    use super::*;
    use crate::test_support::mock_host;

    #[gpui::test]
    fn the_shell_s_choosers_menus_and_dialogs_become_the_desktop_s(cx: &mut TestAppContext) {
        let host = mock_host(&Rc::new(MockPlatform::default()));
        let installed = cx.update(|cx| {
            install(&host, cx);
            (
                cx.has_global::<ui::FileChooser>(),
                cx.has_global::<ui::NativeMenu>(),
                cx.has_global::<ui::NativeAlert>(),
            )
        });
        assert_eq!(installed, (true, true, true));
    }
}
