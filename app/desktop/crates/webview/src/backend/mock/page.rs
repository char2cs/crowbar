use crate::backend::mock::call::Call;
use crate::backend::mock::mock_platform::MockPlatform;
use crate::backend::page_control::PageControl;
use crate::backend::view_capture::ViewCapture;
use crate::error::Result;
use crate::view::Id;
use crate::view::Snapshot;

impl PageControl for MockPlatform {
    fn eval(
        &self,
        id: Id,
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
        id: Id,
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
        ids: &[Id],
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
        let id = Id(3);
        assert!(
            matches!(mock.eval(id, "1"), Err(Error::UnknownWebView(_))),
            "eval"
        );
        assert!(
            matches!(mock.navigate(id, "x"), Err(Error::UnknownWebView(_))),
            "navigate"
        );
        let answers = mock.snapshot_all(&[id, Id(4)]);
        assert_eq!(answers.len(), 2, "one answer per id");
        assert_eq!(
            mock.calls(),
            [Call::SnapshotAll {
                ids: vec![id, Id(4)]
            }],
            "only the snapshot was recorded"
        );
    }
}
