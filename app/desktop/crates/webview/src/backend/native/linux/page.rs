use std::sync::mpsc;
use std::time::Duration;

use crate::backend::native::linux::gtk_platform::GtkPlatform;
use crate::backend::page_control::PageControl;
use crate::backend::view_capture::ViewCapture;
use crate::error::Result;
use crate::view::Id;
use crate::view::Snapshot;

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
