use std::sync::mpsc;

use gtk::gdk;
use gtk::glib::Cast as _;
use gtk::prelude::{GtkWindowExt as _, WidgetExt as _};
use webkit2gtk::{SnapshotOptions, SnapshotRegion, WebViewExt as _};
use wry::WebViewExtUnix;

use crate::backend::native::linux::capture::snapshot_of;
use crate::backend::native::linux::gtk_state::GtkState;
use crate::backend::native::linux::gtk_state::page_focus;
use crate::backend::native::wry_builder::to_rect;
use crate::error::Result;
use crate::view::Bounds;
use crate::view::Color;
use crate::view::Id;
use crate::view::Snapshot;
use crate::view::Visibility;

// PLATFORM: linux wry 0.53 hands WebKitGTK 8-bit channels as 0..1 doubles, so every colour but black reads as white; the view is coloured here instead.
pub(super) fn paint_background(
    view: &wry::WebView,
    color: Color,
) {
    let channel = |value: u8| f64::from(value) / f64::from(u8::MAX);
    view.webview().set_background_color(&gdk::RGBA::new(
        channel(color.red),
        channel(color.green),
        channel(color.blue),
        channel(color.alpha),
    ));
}

// PLATFORM: linux WebKit's script dialog holds the GTK focus inside the view, and grabbing the view would take its typing away.
fn grab_unless_within(view: &gtk::Widget) {
    let window = view
        .toplevel()
        .and_then(|top| top.downcast::<gtk::Window>().ok());
    let within = window
        .and_then(|window| window.focused_widget())
        .is_some_and(|focused| focused.is_ancestor(view));
    if within {
        return;
    }
    view.grab_focus();
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
        paint_background(&self.live(id)?.view, color);
        Ok(())
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
        page_focus::give_keys(self.views.values(), live.page_window);
        grab_unless_within(live.view.webview().upcast_ref());
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
        page_focus::give_keys(self.views.values(), parent);
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
