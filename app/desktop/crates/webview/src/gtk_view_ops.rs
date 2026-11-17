use std::sync::mpsc;

use webkit2gtk::{SnapshotOptions, SnapshotRegion, WebViewExt as _};
use wry::WebViewExtUnix;

use crate::bounds::Bounds;
use crate::builder_support::to_rect;
use crate::color::Color;
use crate::error::Result;
use crate::gtk_state::GtkState;
use crate::snapshot::Snapshot;
use crate::surface_pixels::snapshot_of;
use crate::visibility::Visibility;
use crate::web_view_id::WebViewId;

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
        id: WebViewId,
        bounds: Bounds,
    ) -> Result<()> {
        let live = self.live(id)?;
        live.place.place(&live.view, to_rect(bounds))
    }

    pub(crate) fn set_visibility(
        &self,
        id: WebViewId,
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
        id: WebViewId,
        color: Color,
    ) -> Result<()> {
        Ok(self.live(id)?.view.set_background_color(color.rgba())?)
    }

    pub(crate) fn eval(
        &self,
        id: WebViewId,
        script: &str,
    ) -> Result<()> {
        Ok(self.live(id)?.view.evaluate_script(script)?)
    }

    pub(crate) fn navigate(
        &self,
        id: WebViewId,
        url: &str,
    ) -> Result<()> {
        Ok(self.live(id)?.view.load_url(url)?)
    }

    pub(crate) fn focus(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        Ok(self.live(id)?.view.focus()?)
    }

    pub(crate) fn focus_parent(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        Ok(self.live(id)?.view.focus_parent()?)
    }

    pub(crate) fn snapshot(
        &self,
        id: WebViewId,
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
