use std::path::PathBuf;

use gtk::glib::{Cast as _, IsA};
use gtk::prelude::{DialogExt as _, FileChooserExt as _, GtkWindowExt as _, WidgetExt as _};
use webkit2gtk::{FileChooserRequest, FileChooserRequestExt as _, WebViewExt as _};

const OPEN_ONE: &str = "Select File";
const OPEN_MANY: &str = "Select Files";
const CANCEL: &str = "_Cancel";
const OPEN: &str = "_Open";

// PLATFORM: linux WebKit makes its chooser transient for the page's GtkWindow, reparented into the shell window, so the WM neither centres it on the shell nor returns focus after it.
pub(crate) fn choose_over(
    view: &webkit2gtk::WebView,
    top: u32,
) {
    view.connect_run_file_chooser(move |_, request| {
        ask(request, top);
        true
    });
}

fn ask(
    request: &FileChooserRequest,
    top: u32,
) {
    let dialog = chooser(top, gtk::FileChooserAction::Open);
    dialog.set_title(title(request));
    dialog.set_select_multiple(request.selects_multiple());
    if let Some(filter) = request.mime_types_filter() {
        dialog.set_filter(&filter);
    }
    let response = dialog.run();
    answer(request, &dialog, response);
    dialog.close();
}

fn title(request: &FileChooserRequest) -> &'static str {
    if request.selects_multiple() {
        return OPEN_MANY;
    }
    OPEN_ONE
}

pub(crate) fn chooser(
    top: u32,
    action: gtk::FileChooserAction,
) -> gtk::FileChooserDialog {
    let buttons = [
        (CANCEL, gtk::ResponseType::Cancel),
        (OPEN, gtk::ResponseType::Accept),
    ];
    let dialog = gtk::FileChooserDialog::with_buttons(None, None::<&gtk::Window>, action, &buttons);
    dialog.set_modal(true);
    dialog.realize();
    keep_over(&dialog, top);
    dialog
}

pub(crate) fn keep_over(
    dialog: &impl IsA<gtk::Widget>,
    top: u32,
) {
    let Ok(display) = dialog.display().downcast::<gdkx11::X11Display>() else {
        tracing::warn!("file chooser: not on an X11 display, left to the WM");
        return;
    };
    let Some(window) = dialog.window() else {
        return;
    };
    let shell = gdkx11::X11Window::foreign_new_for_display(&display, top.into());
    window.set_transient_for(shell.upcast_ref());
}

fn answer(
    request: &FileChooserRequest,
    dialog: &gtk::FileChooserDialog,
    response: gtk::ResponseType,
) {
    if response != gtk::ResponseType::Accept {
        return request.cancel();
    }
    let paths = utf8_paths(&dialog.filenames());
    let chosen: Vec<&str> = paths.iter().map(String::as_str).collect();
    request.select_files(&chosen);
}

fn utf8_paths(paths: &[PathBuf]) -> Vec<String> {
    paths
        .iter()
        .filter_map(|path| path.to_str().map(str::to_owned))
        .collect()
}

#[cfg(test)]
mod tests {
    use std::ffi::OsStr;
    use std::os::unix::ffi::OsStrExt as _;

    use super::*;

    #[test]
    fn only_paths_a_page_can_read_as_text_are_handed_to_it() {
        let chosen = [
            PathBuf::from("/home/user/notes.md"),
            PathBuf::from(OsStr::from_bytes(b"/home/user/\xff.bin")),
        ];
        assert_eq!(
            utf8_paths(&chosen),
            ["/home/user/notes.md"],
            "the non-UTF-8 name is left out"
        );
    }
}
