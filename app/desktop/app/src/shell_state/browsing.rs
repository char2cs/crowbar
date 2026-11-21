use panehost::BrowseAction;
use protocol::PaneId;

use super::ShellState;

impl ShellState {
    pub(crate) fn browse(
        &mut self,
        pane: &PaneId,
        action: &BrowseAction,
    ) -> panehost::Result<()> {
        if let BrowseAction::Go(url) = action
            && !self.host.has_view(pane)
        {
            self.save_address(pane.clone(), url.as_str());
            return Ok(());
        }
        self.host.browse(pane, action)
    }
}

#[cfg(test)]
mod tests {
    use browser::PageUrl;
    use protocol::{PaneTypeId, Params};

    use super::*;
    use crate::core_panes::BROWSER;
    use crate::test_support::shell_rig;

    #[test]
    fn going_somewhere_from_a_browser_with_no_page_saves_the_address_its_view_opens_at() {
        let mut rig = shell_rig();
        let browser = protocol::PaneOpen {
            type_id: PaneTypeId::new(BROWSER).unwrap(),
            params: Params::default(),
            file: None,
            mode: protocol::OpenMode::Kept,
        };
        let nobody = PaneId::new("nobody").unwrap();
        rig.shell.open_requested(&nobody, browser);
        let pane = rig.shell.layout().tabs()[0].pane_ids()[0].clone();
        let page = PageUrl::parse("https://example.com/").unwrap();
        let went = rig.shell.browse(&pane, &BrowseAction::Go(page));
        let saved = rig
            .shell
            .layout()
            .pane(&pane)
            .and_then(|instance| instance.state.as_value().get("url").cloned());
        assert!(went.is_ok(), "{went:?}");
        assert_eq!(saved, Some(serde_json::json!("https://example.com/")));
    }
}
