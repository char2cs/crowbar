use std::cell::Cell;

use block2::RcBlock;
use objc2::rc::Retained;
use objc2_app_kit::{NSAlert, NSAlertFirstButtonReturn, NSAlertStyle, NSModalResponse};
use objc2_foundation::{MainThreadMarker, NSString};
use platform::AppkitParent;

use crate::alert::{AlertButton, AlertRequest, AlertRole};
use crate::backend::desktop::alert_asking::{AlertAsking, ChosenButton};
use crate::backend::native::macos::appkit_platform::AppkitPlatform;
use crate::error::{Error, Result};
use crate::view::ParentWindow;

impl AlertAsking for AppkitPlatform {
    fn ask_alert(
        &self,
        parent: ParentWindow,
        request: AlertRequest,
        answer: ChosenButton,
    ) -> Result<()> {
        let ParentWindow::AppKit(address) = parent else {
            return Err(Error::UnsupportedParent(
                "a macOS dialog needs an AppKit window",
            ));
        };
        let window = AppkitParent::find(address, self.main_thread)
            .and_then(|view| view.window())
            .ok_or(Error::UnsupportedParent("no open window shows that NSView"))?;
        let (alert, order) = alert_for(&request, self.main_thread);
        let answer = Cell::new(Some(answer));
        let handler = RcBlock::new(move |response: NSModalResponse| {
            deliver(&answer, pressed(response, &order));
        });
        alert.beginSheetModalForWindow_completionHandler(&window, Some(&handler));
        Ok(())
    }
}

fn alert_for(
    request: &AlertRequest,
    main_thread: MainThreadMarker,
) -> (Retained<NSAlert>, Vec<usize>) {
    let alert = NSAlert::new(main_thread);
    let (text, detail) = match &request.title {
        Some(title) => (title.as_str(), Some(request.message.as_str())),
        None => (request.message.as_str(), None),
    };
    alert.setMessageText(&NSString::from_str(text));
    if let Some(detail) = detail {
        alert.setInformativeText(&NSString::from_str(detail));
    }
    alert.setAlertStyle(NSAlertStyle::Warning);
    let order = shown_order(&request.buttons);
    for button in order.iter().filter_map(|&at| request.buttons.get(at)) {
        let shown = alert.addButtonWithTitle(&NSString::from_str(&button.label));
        shown.setHasDestructiveAction(button.role == AlertRole::Destructive);
    }
    (alert, order)
}

// PLATFORM: macos AppKit makes the first button added the default, rightmost one, and Escape presses Cancel.
fn shown_order(buttons: &[AlertButton]) -> Vec<usize> {
    let accepts = buttons
        .iter()
        .any(|button| button.role == AlertRole::Accept);
    let rank = |button: &AlertButton| match button.role {
        AlertRole::Accept => 0,
        AlertRole::Destructive if !accepts => 0,
        AlertRole::Cancel => 1,
        AlertRole::Destructive => 2,
    };
    let mut order: Vec<usize> = (0..buttons.len()).collect();
    order.sort_by_key(|&at| buttons.get(at).map(rank));
    order
}

fn deliver(
    answer: &Cell<Option<ChosenButton>>,
    pressed: Option<usize>,
) {
    if let Some(answer) = answer.take() {
        answer(pressed);
    }
}

fn pressed(
    response: NSModalResponse,
    order: &[usize],
) -> Option<usize> {
    let shown = usize::try_from(response - NSAlertFirstButtonReturn).ok()?;
    order.get(shown).copied()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn buttons(roles: &[AlertRole]) -> Vec<AlertButton> {
        roles
            .iter()
            .map(|&role| AlertButton {
                label: String::new(),
                role,
            })
            .collect()
    }

    #[test]
    fn the_accepting_button_is_the_default_and_a_lone_destructive_one_takes_its_place() {
        use AlertRole::{Accept, Cancel, Destructive};
        let cases = [
            (
                "save, cancel, discard",
                vec![Cancel, Destructive, Accept],
                vec![2, 0, 1],
            ),
            ("delete, cancel", vec![Cancel, Destructive], vec![1, 0]),
        ];
        for (what, roles, order) in cases {
            assert_eq!(shown_order(&buttons(&roles)), order, "{what}");
        }
    }

    #[test]
    fn the_pressed_button_is_told_by_its_place_in_the_request() {
        let order = [2, 0, 1];
        let second = NSAlertFirstButtonReturn + 1;
        assert_eq!(pressed(second, &order), Some(0), "cancel");
        assert_eq!(pressed(0, &order), None, "dismissed");
    }
}
