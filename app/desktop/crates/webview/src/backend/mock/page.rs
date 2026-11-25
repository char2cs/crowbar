use crate::backend::mock::call::Call;
use crate::backend::mock::mock_platform::MockPlatform;
use crate::backend::view::page_control::PageControl;
use crate::backend::view::view_capture::ViewCapture;
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
    fn snapshot(
        &self,
        id: Id,
    ) -> Result<Option<Snapshot>> {
        self.record(Call::Snapshot { id });
        Ok(None)
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
        assert!(matches!(mock.snapshot(id), Ok(None)), "no pixels");
        assert_eq!(
            mock.calls(),
            [Call::Snapshot { id }],
            "only the snapshot was recorded"
        );
    }
}
