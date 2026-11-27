//! The shell's hooks into the desktop: its own context menus and message dialogs (GTK on the web
//! views' platform thread, `AppKit` on macOS) in place of drawn ones, and on Linux GTK's file
//! choosers and the pop-up window's placer.

mod alert;
#[cfg(target_os = "linux")]
mod file_chooser;
#[cfg(target_os = "linux")]
pub(crate) mod float;
mod menu;

use gpui::App;

pub(crate) fn install(
    host: &panehost::PaneHost,
    cx: &mut App,
) {
    let desktop = host.desktop();
    // PLATFORM: macos gpui's own NSOpenPanel chooses files there.
    #[cfg(target_os = "linux")]
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
    fn menus_and_dialogs_are_the_desktop_s_and_choosers_are_on_linux(cx: &mut TestAppContext) {
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
        assert_eq!(installed, (linux, true, true));
    }
}
