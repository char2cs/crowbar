use gtk::glib::Cast as _;
use gtk::prelude::{DialogExt as _, FileChooserExt as _, GtkWindowExt as _, WidgetExt as _};
use webkit2gtk::{FileChooserRequest, FileChooserRequestExt as _, WebViewExt as _};

const OPEN_ONE: &str = "Select File";
const OPEN_MANY: &str = "Select Files";
const CANCEL: &str = "_Cancel";
const OPEN: &str = "_Open";

// PLATFORM: linux WebKit makes its chooser transient for the page's GtkWindow, which is reparented
// into the shell window, so the WM neither centres it on the shell nor returns focus after it.
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
    let dialog = chooser(top);
    let multiple = request.selects_multiple();
    dialog.set_title(if multiple { OPEN_MANY } else { OPEN_ONE });
    dialog.set_select_multiple(multiple);
    if let Some(filter) = request.mime_types_filter() {
        dialog.set_filter(&filter);
    }
    let response = dialog.run();
    answer(request, &dialog, response);
    dialog.close();
}

fn chooser(top: u32) -> gtk::FileChooserDialog {
    let buttons = [
        (CANCEL, gtk::ResponseType::Cancel),
        (OPEN, gtk::ResponseType::Accept),
    ];
    let dialog = gtk::FileChooserDialog::with_buttons(
        None,
        None::<&gtk::Window>,
        gtk::FileChooserAction::Open,
        &buttons,
    );
    dialog.set_modal(true);
    dialog.realize();
    keep_over(&dialog, top);
    dialog
}

fn keep_over(
    dialog: &gtk::FileChooserDialog,
    top: u32,
) {
    let display = dialog.display().downcast::<gdkx11::X11Display>().ok();
    let (Some(display), Some(window)) = (display, dialog.window()) else {
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
    let paths: Vec<String> = dialog
        .filenames()
        .iter()
        .filter_map(|path| path.to_str().map(str::to_owned))
        .collect();
    let chosen: Vec<&str> = paths.iter().map(String::as_str).collect();
    request.select_files(&chosen);
}

#[cfg(test)]
pub(super) mod tests {
    use x11rb::protocol::xproto::{AtomEnum, ConnectionExt as _};

    use super::*;

    fn xid(window: &gtk::gdk::Window) -> u32 {
        window
            .clone()
            .downcast::<gdkx11::X11Window>()
            .ok()
            .and_then(|window| u32::try_from(window.xid()).ok())
            .unwrap()
    }

    fn transient_for(window: u32) -> Option<u32> {
        let (connection, _) = x11rb::connect(None).unwrap();
        let reply = connection
            .get_property(
                false,
                window,
                AtomEnum::WM_TRANSIENT_FOR,
                AtomEnum::WINDOW,
                0,
                1,
            )
            .unwrap()
            .reply()
            .unwrap();
        reply.value32().and_then(|mut values| values.next())
    }

    pub(in crate::backend::native::linux) fn the_chooser_is_transient_for_the_shell_window() {
        let shell = gtk::Window::new(gtk::WindowType::Toplevel);
        shell.show_all();
        let top = xid(&shell.window().unwrap());
        let dialog = chooser(top);
        gtk::gdk::Display::default().unwrap().sync();
        assert_eq!(transient_for(xid(&dialog.window().unwrap())), Some(top));
    }
}
