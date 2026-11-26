//! Opens develop's GTK chooser for the shell's buttons, on the web views' platform thread.

use std::path::PathBuf;
use std::rc::Rc;

use gpui::{App, Task, Window};

pub(super) fn install(
    chooser: webview::Desktop,
    cx: &mut App,
) {
    let choose: ui::ChooseFiles =
        Rc::new(move |choice, window, cx| ask((&chooser, web_choice(choice)), window, cx));
    cx.set_global(ui::FileChooser::new(choose));
}

fn ask(
    (chooser, choice): (&webview::Desktop, webview::FileChoice),
    window: &Window,
    cx: &mut App,
) -> Task<Vec<PathBuf>> {
    let (sender, answer) = async_channel::bounded(1);
    let send: webview::ChosenFiles = Box::new(move |paths| {
        sender.try_send(paths).unwrap_or_default();
    });
    let asked = webview::ParentWindow::from_window(window)
        .and_then(|parent| chooser.choose(parent, choice, send));
    if let Err(error) = asked {
        tracing::warn!(%error, "the file chooser could not open");
    }
    cx.background_executor()
        .spawn(async move { answer.recv().await.unwrap_or_default() })
}

fn web_choice(choice: ui::FileChoice) -> webview::FileChoice {
    webview::FileChoice {
        title: choice.title,
        kind: web_kind(choice.kind),
        filter: choice.filter.map(web_filter),
    }
}

fn web_kind(kind: ui::ChoiceKind) -> webview::ChoiceKind {
    match kind {
        ui::ChoiceKind::File => webview::ChoiceKind::File,
        ui::ChoiceKind::Files => webview::ChoiceKind::Files,
        ui::ChoiceKind::Folder => webview::ChoiceKind::Folder,
    }
}

fn web_filter(filter: ui::FileFilter) -> webview::FileFilter {
    webview::FileFilter {
        name: filter.name,
        extensions: filter.extensions,
        mime_types: filter.mime_types,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const IMAGES: ui::FileFilter = ui::FileFilter {
        name: Some("Images"),
        extensions: &["png", "jpg"],
        mime_types: &["image/webp"],
    };

    const CHOICES: [(ui::FileChoice, webview::FileChoice); 2] = [
        (
            ui::FileChoice {
                title: "Open File",
                kind: ui::ChoiceKind::File,
                filter: Some(IMAGES),
            },
            webview::FileChoice {
                title: "Open File",
                kind: webview::ChoiceKind::File,
                filter: Some(webview::FileFilter {
                    name: Some("Images"),
                    extensions: &["png", "jpg"],
                    mime_types: &["image/webp"],
                }),
            },
        ),
        (
            ui::FileChoice {
                title: "Select Folder",
                kind: ui::ChoiceKind::Folder,
                filter: None,
            },
            webview::FileChoice {
                title: "Select Folder",
                kind: webview::ChoiceKind::Folder,
                filter: None,
            },
        ),
    ];

    #[test]
    fn the_shell_s_choice_reaches_the_web_view_chooser_unchanged() {
        for (asked, opened) in CHOICES {
            assert_eq!(web_choice(asked), opened, "{asked:?}");
        }
    }
}
