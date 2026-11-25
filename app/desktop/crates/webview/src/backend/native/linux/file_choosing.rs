//! The shell's own file choosers: develop's GTK dialog, with each caller's title and filter.

use std::path::PathBuf;

use gtk::glib;
use gtk::prelude::{DialogExtManual as _, FileChooserExt as _, GtkWindowExt as _};

use crate::backend::desktop::file_choosing::{ChosenFiles, FileChoosing};
use crate::backend::native::linux::gtk_platform::GtkPlatform;
use crate::backend::native::linux::gtk_state::chooser;
use crate::chooser::{ChoiceKind, FileChoice, FileFilter};
use crate::error::{Error, Result};
use crate::view::ParentWindow;

impl FileChoosing for GtkPlatform {
    fn choose_files(
        &self,
        parent: ParentWindow,
        choice: FileChoice,
        answer: ChosenFiles,
    ) -> Result<()> {
        let ParentWindow::X11(top) = parent else {
            return Err(Error::UnsupportedParent(
                "a Linux chooser needs an X11 window",
            ));
        };
        self.post(move |_| {
            glib::MainContext::default().spawn_local(ask(choice, top, answer));
        })
    }
}

async fn ask(
    choice: FileChoice,
    top: u32,
    answer: ChosenFiles,
) {
    let dialog = dialog_for(&choice, top);
    let response = dialog.run_future().await;
    answer(chosen(&dialog, response));
    dialog.close();
}

fn dialog_for(
    choice: &FileChoice,
    top: u32,
) -> gtk::FileChooserDialog {
    let dialog = chooser(top, action_of(choice.kind));
    dialog.set_title(choice.title);
    dialog.set_select_multiple(choice.kind == ChoiceKind::Files);
    if let Some(filter) = choice.filter {
        apply(&dialog, filter);
    }
    dialog
}

fn action_of(kind: ChoiceKind) -> gtk::FileChooserAction {
    match kind {
        ChoiceKind::File | ChoiceKind::Files => gtk::FileChooserAction::Open,
        ChoiceKind::Folder => gtk::FileChooserAction::SelectFolder,
    }
}

// PLATFORM: linux a named filter is rfd's (listed in the filter menu); an unnamed one is WebKit's for a page's `accept` (no menu).
fn apply(
    dialog: &gtk::FileChooserDialog,
    filter: FileFilter,
) {
    let files = gtk::FileFilter::new();
    for extension in filter.extensions {
        files.add_pattern(&format!("*.{extension}"));
    }
    for mime_type in filter.mime_types {
        files.add_mime_type(mime_type);
    }
    let Some(name) = filter.name else {
        return dialog.set_filter(&files);
    };
    files.set_name(Some(name));
    dialog.add_filter(files);
}

fn chosen(
    dialog: &gtk::FileChooserDialog,
    response: gtk::ResponseType,
) -> Vec<PathBuf> {
    if response != gtk::ResponseType::Accept {
        return Vec::new();
    }
    dialog.filenames()
}

#[cfg(test)]
mod tests {
    use super::*;

    const ACTIONS: [(ChoiceKind, gtk::FileChooserAction); 3] = [
        (ChoiceKind::File, gtk::FileChooserAction::Open),
        (ChoiceKind::Files, gtk::FileChooserAction::Open),
        (ChoiceKind::Folder, gtk::FileChooserAction::SelectFolder),
    ];

    const PARENTS: [(ParentWindow, usize); 2] =
        [(ParentWindow::X11(1), 1), (ParentWindow::AppKit(1), 0)];

    #[test]
    fn a_folder_is_chosen_with_gtk_s_folder_action_and_files_with_its_open_action() {
        for (kind, action) in ACTIONS {
            assert_eq!(action_of(kind), action, "{kind:?}");
        }
    }

    fn post_over(parent: ParentWindow) -> (bool, usize) {
        let (jobs, inbox) = async_channel::unbounded();
        let platform = GtkPlatform {
            jobs,
            stopped: None,
        };
        let choice = FileChoice {
            title: "Open File",
            kind: ChoiceKind::File,
            filter: None,
        };
        let posted = platform.choose_files(parent, choice, Box::new(|_| ()));
        (posted.is_ok(), inbox.len())
    }

    #[test]
    fn only_a_chooser_over_an_x11_window_is_posted_to_the_gtk_thread() {
        for (parent, jobs_posted) in PARENTS {
            let want = (jobs_posted == 1, jobs_posted);
            assert_eq!(post_over(parent), want, "{parent:?}");
        }
    }
}
