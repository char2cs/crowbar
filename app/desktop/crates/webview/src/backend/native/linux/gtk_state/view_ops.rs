use std::sync::mpsc;

use gtk::gdk;
use gtk::gdk::EventMask;
use gtk::prelude::WidgetExt as _;
use webkit2gtk::{SnapshotOptions, SnapshotRegion, WebViewExt as _};
use wry::WebViewExtUnix;

use crate::backend::native::linux::capture::snapshot_of;
use crate::backend::native::linux::gtk_state::GtkState;
use crate::backend::native::linux::gtk_state::live_view::LiveView;
use crate::backend::native::wry_builder::to_rect;
use crate::error::Result;
use crate::view::Bounds;
use crate::view::Color;
use crate::view::Id;
use crate::view::Snapshot;
use crate::view::Visibility;

const KEYS: EventMask = EventMask::KEY_PRESS_MASK.union(EventMask::KEY_RELEASE_MASK);

// PLATFORM: linux X hands a key to the deepest window under the pointer that selects keys while the pointer lies inside the focus window, so only the page holding the keyboard selects them.
fn select_keys(
    live: &LiveView,
    keys: impl FnOnce(EventMask) -> EventMask,
) {
    if let Some(window) = live.view.webview().window() {
        window.set_events(keys(window.events()));
    }
}

pub(crate) fn deactivate(live: &LiveView) {
    select_keys(live, |events| events - KEYS);
}

fn answer(
    reply: &mpsc::Sender<Result<Option<Snapshot>>>,
    result: Result<Option<Snapshot>>,
) {
    if reply.send(result).is_err() {
        tracing::debug!("a snapshot arrived after its caller gave up");
    }
}

impl GtkState {
    pub(crate) fn set_bounds(
        &self,
        id: Id,
        bounds: Bounds,
    ) -> Result<()> {
        let live = self.live(id)?;
        live.place.place(&live.view, to_rect(bounds))
    }

    pub(crate) fn set_visibility(
        &self,
        id: Id,
        visibility: Visibility,
    ) -> Result<()> {
        let live = self.live(id)?;
        match visibility {
            Visibility::Hidden => Ok(live.view.set_visible(false)?),
            Visibility::Shown => live.place.show(&live.view),
        }
    }

    pub(crate) fn set_background(
        &self,
        id: Id,
        color: Color,
    ) -> Result<()> {
        Ok(self.live(id)?.view.set_background_color(color.rgba())?)
    }

    pub(crate) fn eval(
        &self,
        id: Id,
        script: &str,
    ) -> Result<()> {
        Ok(self.live(id)?.view.evaluate_script(script)?)
    }

    pub(crate) fn navigate(
        &self,
        id: Id,
        url: &str,
    ) -> Result<()> {
        Ok(self.live(id)?.view.load_url(url)?)
    }

    pub(crate) fn focus(
        &self,
        id: Id,
    ) -> Result<()> {
        let live = self.live(id)?;
        self.views
            .iter()
            .filter(|(other, _)| **other != id)
            .for_each(|(_, other)| deactivate(other));
        select_keys(live, |events| events | KEYS);
        live.view.focus()?;
        // PLATFORM: linux X serves each client connection on its own, so GDK's queued map of the page window must reach the server before the focus request on ours.
        gdk::Display::default().iter().for_each(gdk::Display::sync);
        self.keyboard.give_to(live.page_window)?;
        self.follow_focus();
        Ok(())
    }

    pub(crate) fn focus_parent(
        &self,
        id: Id,
    ) -> Result<()> {
        let parent = self.live(id)?.parent;
        self.views.values().for_each(deactivate);
        self.keyboard.give_to(parent)?;
        self.follow_focus();
        Ok(())
    }

    pub(crate) fn snapshot(
        &self,
        id: Id,
        reply: mpsc::Sender<Result<Option<Snapshot>>>,
    ) {
        let page = match self.live(id) {
            Ok(live) => live.view.webview(),
            Err(error) => return answer(&reply, Err(error)),
        };
        page.snapshot(
            SnapshotRegion::Visible,
            SnapshotOptions::NONE,
            None::<&gtk::gio::Cancellable>,
            move |surface| answer(&reply, Ok(surface.ok().and_then(snapshot_of))),
        );
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn an_answer_reaches_a_waiting_caller_and_a_late_one_is_dropped() {
        let (reply, answers) = mpsc::channel();
        answer(&reply, Ok(None));
        assert!(matches!(answers.try_recv(), Ok(Ok(None))), "delivered");
        drop(answers);
        answer(&reply, Ok(None));
    }
}
