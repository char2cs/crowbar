use gtk::glib::{self, prelude::ObjectExt as _};
use gtk::prelude::{
    DialogExt as _, DialogExtManual as _, GtkWindowExt as _, MessageDialogExt as _,
    StyleContextExt as _, WidgetExt as _,
};

use crate::alert::{AlertButton, AlertRequest, AlertRole};
use crate::backend::desktop::alert_asking::{AlertAsking, ChosenButton};
use crate::backend::native::linux::gtk_platform::GtkPlatform;
use crate::backend::native::linux::gtk_state::keep_over;
use crate::error::{Error, Result};
use crate::view::ParentWindow;

const DESTRUCTIVE: &str = "destructive-action";
const SUGGESTED: &str = "suggested-action";

impl AlertAsking for GtkPlatform {
    fn ask_alert(
        &self,
        parent: ParentWindow,
        request: AlertRequest,
        answer: ChosenButton,
    ) -> Result<()> {
        let ParentWindow::X11(top) = parent else {
            return Err(Error::UnsupportedParent(
                "a Linux dialog needs an X11 window",
            ));
        };
        self.post(move |_| {
            glib::MainContext::default().spawn_local(ask(request, top, answer));
        })
    }
}

async fn ask(
    request: AlertRequest,
    top: u32,
    answer: ChosenButton,
) {
    let dialog = dialog_for(&request);
    dialog.realize();
    keep_over(&dialog, top);
    let response = dialog.run_future().await;
    answer(pressed(response, request.buttons.len()));
    dialog.close();
}

fn dialog_for(request: &AlertRequest) -> gtk::MessageDialog {
    let (text, secondary) = match &request.title {
        Some(title) => (title.as_str(), Some(request.message.as_str())),
        None => (request.message.as_str(), None),
    };
    let dialog = gtk::MessageDialog::new(
        None::<&gtk::Window>,
        gtk::DialogFlags::MODAL,
        gtk::MessageType::Warning,
        gtk::ButtonsType::None,
        text,
    );
    dialog.set_secondary_text(secondary);
    for (index, button) in request.buttons.iter().enumerate() {
        add(&dialog, (index, button));
    }
    if let Some(default) = default_of(&request.buttons) {
        dialog.set_default_response(response_of(default));
    }
    dialog
}

fn add(
    dialog: &gtk::MessageDialog,
    (index, button): (usize, &AlertButton),
) {
    let widget = dialog.add_button(&button.label, response_of(index));
    widget.set_property("use-underline", false);
    let class = match button.role {
        AlertRole::Cancel => return,
        AlertRole::Accept => SUGGESTED,
        AlertRole::Destructive => DESTRUCTIVE,
    };
    widget.style_context().add_class(class);
}

// PLATFORM: linux Enter presses the Accept button; with only a dangerous one it stays on Cancel, as develop's initial focus does.
fn default_of(buttons: &[AlertButton]) -> Option<usize> {
    let with = |role: AlertRole| buttons.iter().position(|button| button.role == role);
    with(AlertRole::Accept).or_else(|| with(AlertRole::Cancel))
}

fn response_of(index: usize) -> gtk::ResponseType {
    u16::try_from(index).map_or(gtk::ResponseType::None, gtk::ResponseType::Other)
}

fn pressed(
    response: gtk::ResponseType,
    buttons: usize,
) -> Option<usize> {
    let gtk::ResponseType::Other(index) = response else {
        return None;
    };
    Some(usize::from(index)).filter(|index| *index < buttons)
}

#[cfg(test)]
mod tests {
    use super::*;

    const PARENTS: [(ParentWindow, usize); 2] =
        [(ParentWindow::X11(1), 1), (ParentWindow::AppKit(1), 0)];

    fn button(
        label: &str,
        role: AlertRole,
    ) -> AlertButton {
        AlertButton {
            label: label.into(),
            role,
        }
    }

    fn post_over(parent: ParentWindow) -> (bool, usize) {
        let (jobs, inbox) = async_channel::unbounded();
        let platform = GtkPlatform {
            jobs,
            stopped: None,
        };
        let request = AlertRequest {
            title: None,
            message: "Reload?".into(),
            buttons: vec![],
        };
        let posted = platform.ask_alert(parent, request, Box::new(|_| ()));
        (posted.is_ok(), inbox.len())
    }

    #[test]
    fn only_a_dialog_over_an_x11_window_is_posted_to_the_gtk_thread() {
        for (parent, jobs_posted) in PARENTS {
            let want = (jobs_posted == 1, jobs_posted);
            assert_eq!(post_over(parent), want, "{parent:?}");
        }
    }

    type DefaultCase = (&'static [AlertRole], Option<usize>);

    const DEFAULT_CASES: [DefaultCase; 4] = [
        (&[AlertRole::Cancel, AlertRole::Accept], Some(1)),
        (
            &[AlertRole::Cancel, AlertRole::Destructive, AlertRole::Accept],
            Some(2),
        ),
        (&[AlertRole::Cancel, AlertRole::Destructive], Some(0)),
        (&[], None),
    ];

    #[test]
    fn enter_goes_ahead_unless_the_only_way_ahead_loses_something() {
        for (roles, want) in DEFAULT_CASES {
            let buttons: Vec<AlertButton> = roles.iter().map(|role| button("B", *role)).collect();
            assert_eq!(default_of(&buttons), want, "{roles:?}");
        }
    }

    const PRESS_CASES: [(gtk::ResponseType, Option<usize>); 3] = [
        (gtk::ResponseType::Other(1), Some(1)),
        (gtk::ResponseType::DeleteEvent, None),
        (gtk::ResponseType::Other(7), None),
    ];

    #[test]
    fn only_a_pressed_button_answers_and_escape_or_closing_dismisses() {
        for (response, want) in PRESS_CASES {
            assert_eq!(pressed(response, 2), want, "{response:?}");
        }
    }
}
