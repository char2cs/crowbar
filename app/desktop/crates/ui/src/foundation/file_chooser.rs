//! The desktop's file chooser the shell's buttons open; the shell installs it at start.

mod choice_kind;
mod file_choice;
mod file_filter;

use std::path::PathBuf;
use std::rc::Rc;

use gpui::{App, Global, PathPromptOptions, Task, Window};

pub use choice_kind::ChoiceKind;
pub use file_choice::FileChoice;
pub use file_filter::FileFilter;

/// Opens a chooser for a [`FileChoice`] over a window; its task ends with the chosen paths,
/// none when cancelled.
pub type ChooseFiles = Rc<dyn Fn(FileChoice, &Window, &mut App) -> Task<Vec<PathBuf>>>;

/// The installed file chooser, a global; gpui's own path prompt until one is installed.
#[derive(Clone)]
pub struct FileChooser(ChooseFiles);

crate::opaque_debug!(FileChooser);

impl Global for FileChooser {}

impl FileChooser {
    /// A chooser that answers with `choose`.
    #[must_use]
    pub fn new(choose: ChooseFiles) -> Self {
        Self(choose)
    }

    /// Asks for `choice` over `window`; the task ends with the chosen paths.
    pub fn choose(
        choice: FileChoice,
        window: &Window,
        cx: &mut App,
    ) -> Task<Vec<PathBuf>> {
        let Some(chooser) = cx.try_global::<Self>().cloned() else {
            return prompt(choice, cx);
        };
        (chooser.0)(choice, window, cx)
    }
}

// PLATFORM: macos keeps gpui's NSOpenPanel; the Linux shell installs develop's GTK chooser instead.
fn prompt(
    choice: FileChoice,
    cx: &mut App,
) -> Task<Vec<PathBuf>> {
    let picked = cx.prompt_for_paths(PathPromptOptions {
        files: choice.kind != ChoiceKind::Folder,
        directories: choice.kind == ChoiceKind::Folder,
        multiple: choice.kind == ChoiceKind::Files,
        prompt: None,
    });
    cx.background_executor().spawn(async move {
        let answer = picked.await.ok().and_then(Result::ok).flatten();
        answer.unwrap_or_default()
    })
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;

    use gpui::{Context, IntoElement, Render, TestAppContext, div};

    use super::*;

    const FOLDER: FileChoice = FileChoice {
        title: "Select Folder",
        kind: ChoiceKind::Folder,
        filter: None,
    };

    struct Blank;

    impl Render for Blank {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            div()
        }
    }

    #[gpui::test]
    fn the_installed_chooser_is_asked_in_place_of_gpui_s_prompt(cx: &mut TestAppContext) {
        let asked = Rc::new(RefCell::new(Vec::new()));
        let heard = Rc::clone(&asked);
        let choose: ChooseFiles = Rc::new(move |choice, _, _| {
            heard.borrow_mut().push(choice);
            Task::ready(vec![PathBuf::from("/w/app")])
        });
        cx.update(|cx| cx.set_global(FileChooser::new(choose)));
        let (_, window) = cx.add_window_view(|_, _| Blank);
        let task = window.update(|window, cx| FileChooser::choose(FOLDER, window, cx));
        let paths = cx.foreground_executor().block_test(task);
        assert_eq!(*asked.borrow(), [FOLDER]);
        assert_eq!(paths, [PathBuf::from("/w/app")]);
        assert!(!cx.did_prompt_for_paths(), "gpui's prompt stays shut");
    }
}
