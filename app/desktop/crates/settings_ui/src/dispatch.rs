//! Sends an [`Intent`] from an event handler to the modal.

use gpui::{App, Entity, Window};

use crate::intent::Intent;
use crate::settings_modal::SettingsModal;

pub(crate) fn deliver(
    owner: &Entity<SettingsModal>,
    intent: Intent,
    (window, cx): (&mut Window, &mut App),
) {
    owner.update(cx, |modal, cx| modal.run(intent, window, cx));
}

pub(crate) fn send<E>(
    owner: &Entity<SettingsModal>,
    intent: Intent,
) -> impl Fn(&E, &mut Window, &mut App) + 'static {
    let owner = owner.clone();
    move |_, window, cx| deliver(&owner, intent.clone(), (window, cx))
}

#[cfg(test)]
#[path = "dispatch_tests.rs"]
mod tests;
