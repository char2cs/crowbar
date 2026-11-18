use std::sync::mpsc;
use std::time::{Duration, Instant};

use crate::backend::native::linux::bounded_wait::collect_within;
use crate::backend::native::linux::linux_platform::LinuxPlatform;
use crate::backend::page_control::PageControl;
use crate::backend::view_capture::ViewCapture;
use crate::error::Result;
use crate::view::Snapshot;
use crate::view::WebViewId;

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

impl LinuxPlatform {
    fn ask_snapshot(
        &self,
        id: WebViewId,
    ) -> (Result<()>, SnapshotAnswer) {
        let (reply, answer) = mpsc::channel();
        (self.post(move |state| state.snapshot(id, reply)), answer)
    }
}

impl PageControl for LinuxPlatform {
    fn eval(
        &self,
        id: WebViewId,
        script: &str,
    ) -> Result<()> {
        let script = script.to_owned();
        self.post_logged("eval", move |state| state.eval(id, &script))
    }

    fn navigate(
        &self,
        id: WebViewId,
        url: &str,
    ) -> Result<()> {
        let url = url.to_owned();
        self.post_logged("navigate", move |state| state.navigate(id, &url))
    }
}

impl ViewCapture for LinuxPlatform {
    fn snapshot(
        &self,
        id: WebViewId,
    ) -> Result<Option<Snapshot>> {
        self.snapshot_all(&[id]).pop().unwrap_or(Ok(None))
    }

    fn snapshot_all(
        &self,
        ids: &[WebViewId],
    ) -> Vec<Result<Option<Snapshot>>> {
        let (posted, answers): (Vec<_>, Vec<_>) =
            ids.iter().map(|id| self.ask_snapshot(*id)).unzip();
        let replies = collect_within(&answers, SNAPSHOT_PATIENCE, Instant::now);
        posted
            .into_iter()
            .zip(replies)
            .map(|(posted, reply)| settle_snapshot(posted, reply))
            .collect()
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
