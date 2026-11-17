use crate::call::Call;
use crate::error::Result;
use crate::mock_platform::MockPlatform;
use crate::page_control::PageControl;
use crate::snapshot::Snapshot;
use crate::view_capture::ViewCapture;
use crate::web_view_id::WebViewId;

impl PageControl for MockPlatform {
    fn eval(
        &self,
        id: WebViewId,
        script: &str,
    ) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::Eval {
            id,
            script: script.to_owned(),
        });
        Ok(())
    }

    fn navigate(
        &self,
        id: WebViewId,
        url: &str,
    ) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::Navigate {
            id,
            url: url.to_owned(),
        });
        Ok(())
    }
}

impl ViewCapture for MockPlatform {
    fn snapshot_all(
        &self,
        ids: &[WebViewId],
    ) -> Vec<Result<Option<Snapshot>>> {
        self.record(Call::SnapshotAll { ids: ids.to_vec() });
        ids.iter().map(|_| Ok(None)).collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::error::Error;

    #[test]
    fn scripts_and_navigation_need_a_live_view_and_snapshots_are_recorded() {
        let mock = MockPlatform::default();
        let id = WebViewId(3);
        assert!(
            matches!(mock.eval(id, "1"), Err(Error::UnknownWebView(_))),
            "eval"
        );
        assert!(
            matches!(mock.navigate(id, "x"), Err(Error::UnknownWebView(_))),
            "navigate"
        );
        let answers = mock.snapshot_all(&[id, WebViewId(4)]);
        assert_eq!(answers.len(), 2, "one answer per id");
        assert_eq!(
            mock.calls(),
            [Call::SnapshotAll {
                ids: vec![id, WebViewId(4)]
            }],
            "only the snapshot was recorded"
        );
    }
}
