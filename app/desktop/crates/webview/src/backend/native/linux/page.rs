use std::sync::mpsc;
use std::time::Duration;

use crate::backend::native::linux::gtk_platform::GtkPlatform;
use crate::backend::view::page_control::PageControl;
use crate::backend::view::view_capture::ViewCapture;
use crate::backend::view::view_cutout::ViewCutout;
use crate::error::Result;
use crate::view::Id;
use crate::view::Snapshot;
use crate::view::{Bounds, Corners};

const SNAPSHOT_PATIENCE: Duration = Duration::from_millis(400);

type SnapshotAnswer = mpsc::Receiver<Result<Option<Snapshot>>>;

fn settle_snapshot(
    posted: Result<()>,
    reply: Option<Result<Option<Snapshot>>>,
) -> Result<Option<Snapshot>> {
    posted?;
    reply.unwrap_or_else(|| {
        tracing::debug!("a web view did not answer a snapshot in time");
        Ok(None)
    })
}

impl GtkPlatform {
    fn ask_snapshot(
        &self,
        id: Id,
    ) -> (Result<()>, SnapshotAnswer) {
        let (reply, answer) = mpsc::channel();
        (self.post(move |state| state.snapshot(id, reply)), answer)
    }
}

impl PageControl for GtkPlatform {
    fn eval(
        &self,
        id: Id,
        script: &str,
    ) -> Result<()> {
        let script = script.to_owned();
        self.post_logged("eval", move |state| state.eval(id, &script))
    }

    fn navigate(
        &self,
        id: Id,
        url: &str,
    ) -> Result<()> {
        let url = url.to_owned();
        self.post_logged("navigate", move |state| state.navigate(id, &url))
    }
}

impl ViewCutout for GtkPlatform {
    fn set_cutout(
        &self,
        id: Id,
        hole: Option<Bounds>,
    ) -> Result<()> {
        self.post_logged("set_cutout", move |state| state.set_cutout(id, hole))
    }

    fn set_corners(
        &self,
        id: Id,
        corners: Corners,
    ) -> Result<()> {
        self.post_logged("set_corners", move |state| state.set_corners(id, corners))
    }
}

impl ViewCapture for GtkPlatform {
    fn snapshot(
        &self,
        id: Id,
    ) -> Result<Option<Snapshot>> {
        let (posted, answer) = self.ask_snapshot(id);
        settle_snapshot(posted, answer.recv_timeout(SNAPSHOT_PATIENCE).ok())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::error::Error;

    #[test]
    fn a_snapshot_settles_to_the_reply_or_to_nothing_when_nobody_answered() {
        assert!(
            matches!(settle_snapshot(Ok(()), None), Ok(None)),
            "silent view"
        );
        assert!(
            matches!(
                settle_snapshot(Err(Error::HostStopped), None),
                Err(Error::HostStopped)
            ),
            "stopped backend"
        );
        assert!(
            matches!(settle_snapshot(Ok(()), Some(Ok(None))), Ok(None)),
            "answered"
        );
    }
}
