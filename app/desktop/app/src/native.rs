//! The shell's hooks into the desktop: its own message dialogs (GTK on the web views' platform
//! thread, `NSAlert` on macOS) in place of drawn ones, the pop-up window's placer, and on Linux
//! GTK's menus and file choosers.

mod alert;
#[cfg(target_os = "linux")]
mod file_chooser;
pub(crate) mod float;
#[cfg(target_os = "linux")]
mod menu;

use gpui::App;

pub(crate) fn install(
    host: &panehost::PaneHost,
    cx: &mut App,
) {
    let desktop = host.desktop();
    // PLATFORM: macos gpui's own NSOpenPanel chooses files and the shell draws its menus there.
    #[cfg(target_os = "linux")]
    {
        file_chooser::install(desktop.clone(), cx);
        menu::install(desktop.clone(), cx);
    }
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
        let linux = cfg!(target_os = "linux");
        assert_eq!(installed, (linux, linux, true));
    }
}
