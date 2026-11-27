use std::rc::Rc;

use gpui::{App, Task, Window};

pub(super) fn install(
    alert: webview::Desktop,
    cx: &mut App,
) {
    let ask: ui::AskAlert = Rc::new(move |request, window, cx| ask(&alert, &request, (window, cx)));
    cx.set_global(ui::NativeAlert::new(ask));
}

fn ask(
    alert: &webview::Desktop,
    request: &ui::AlertRequest,
    (window, cx): (&Window, &mut App),
) -> Task<Option<usize>> {
    let (sender, answer) = async_channel::bounded(1);
    let send: webview::ChosenButton = Box::new(move |button| {
        sender.try_send(button).unwrap_or_default();
    });
    let asked = webview::ParentWindow::from_window(window)
        .and_then(|parent| alert.ask(parent, web_request(request), send));
    if let Err(error) = asked {
        tracing::warn!(%error, "the message dialog could not open");
    }
    cx.background_executor()
        .spawn(async move { answer.recv().await.unwrap_or_default() })
}

fn web_request(request: &ui::AlertRequest) -> webview::AlertRequest {
    webview::AlertRequest {
        title: request.title.as_ref().map(ToString::to_string),
        message: request.message.to_string(),
        buttons: request.buttons.iter().map(web_button).collect(),
    }
}

fn web_button(button: &ui::AlertButton) -> webview::AlertButton {
    webview::AlertButton {
        label: button.label.to_string(),
        role: match button.role {
            ui::AlertRole::Cancel => webview::AlertRole::Cancel,
            ui::AlertRole::Accept => webview::AlertRole::Accept,
            ui::AlertRole::Destructive => webview::AlertRole::Destructive,
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_shell_s_dialog_reaches_the_web_view_dialog_unchanged() {
        let asked = ui::AlertRequest {
            title: Some("Unsaved Changes".into()),
            message: "a.rs has unsaved changes.".into(),
            buttons: vec![
                ui::AlertButton::new("Cancel", ui::AlertRole::Cancel),
                ui::AlertButton::new("Discard", ui::AlertRole::Destructive),
                ui::AlertButton::new("Save", ui::AlertRole::Accept),
            ],
        };
        let button = |label: &str, role| webview::AlertButton {
            label: label.into(),
            role,
        };
        let opened = webview::AlertRequest {
            title: Some("Unsaved Changes".into()),
            message: "a.rs has unsaved changes.".into(),
            buttons: vec![
                button("Cancel", webview::AlertRole::Cancel),
                button("Discard", webview::AlertRole::Destructive),
                button("Save", webview::AlertRole::Accept),
            ],
        };
        assert_eq!(web_request(&asked), opened);
    }
}
