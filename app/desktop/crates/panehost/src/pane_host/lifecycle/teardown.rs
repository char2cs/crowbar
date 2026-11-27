use crate::error::Result;
use crate::pane_host::PaneHost;

impl PaneHost {
    pub fn close_web_views(&mut self) -> Result<()> {
        self.panes.clear();
        self.pool.clear();
        Ok(self.factory.settle()?)
    }
}

#[cfg(test)]
mod tests {
    use webview::Call;

    use crate::test_support::*;

    #[test]
    fn every_web_view_is_destroyed_before_the_platform_settles() {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        assert_eq!(f.host.web_view_count(), 1);
        f.host.close_web_views().unwrap();
        assert_eq!(f.host.web_view_count(), 0);
        let calls = f.mock.calls();
        let destroyed = calls
            .iter()
            .position(|call| matches!(call, Call::Destroy { .. }));
        let settled = calls.iter().position(|call| matches!(call, Call::Settle));
        assert!(destroyed.is_some(), "destroyed: {calls:?}");
        assert!(destroyed < settled, "then settled: {calls:?}");
    }
}
