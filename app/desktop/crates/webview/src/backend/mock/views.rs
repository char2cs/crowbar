use crate::backend::browsing_data::BrowsingData;
use crate::backend::mock::call::Call;
use crate::backend::mock::mock_platform::MockPlatform;
use crate::backend::view_focus::ViewFocus;
use crate::backend::view_layout::ViewLayout;
use crate::backend::view_lifecycle::ViewLifecycle;
use crate::error::{Error, Result};
use crate::view::Bounds;
use crate::view::Color;
use crate::view::ParentWindow;
use crate::view::Visibility;
use crate::view::WebViewConfig;
use crate::view::WebViewId;

impl ViewLifecycle for MockPlatform {
    fn create(
        &self,
        id: WebViewId,
        parent: ParentWindow,
        config: WebViewConfig,
    ) -> Result<()> {
        if self.fail_next_create.replace(false) {
            return Err(Error::HostStopped);
        }
        self.record(Call::Create {
            id,
            parent,
            url: config.url,
            kind: config.kind,
            bounds: config.bounds,
            visibility: config.visibility,
            background: config.background,
        });
        self.live.borrow_mut().insert(id, config.on_ipc);
        Ok(())
    }

    fn destroy(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        self.live
            .borrow_mut()
            .remove(&id)
            .ok_or(Error::UnknownWebView(id))?;
        self.record(Call::Destroy { id });
        Ok(())
    }
}

impl ViewLayout for MockPlatform {
    fn set_bounds(
        &self,
        id: WebViewId,
        bounds: Bounds,
    ) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::SetBounds { id, bounds });
        Ok(())
    }

    fn set_visibility(
        &self,
        id: WebViewId,
        visibility: Visibility,
    ) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::SetVisibility { id, visibility });
        Ok(())
    }

    fn set_background(
        &self,
        id: WebViewId,
        color: Color,
    ) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::SetBackground { id, color });
        Ok(())
    }
}

impl ViewFocus for MockPlatform {
    fn focus(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::Focus { id });
        Ok(())
    }

    fn focus_parent(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::FocusParent { id });
        Ok(())
    }
}

impl BrowsingData for MockPlatform {
    fn clear_browsing_data(&self) -> Result<()> {
        self.record(Call::ClearBrowsingData);
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::view::WebViewKind;

    fn config() -> WebViewConfig {
        let bounds = Bounds {
            x: 0.0,
            y: 0.0,
            width: 10.0,
            height: 10.0,
        };
        WebViewConfig {
            url: "crowbar://local/".into(),
            bounds,
            kind: WebViewKind::Core,
            visibility: Visibility::Shown,
            background: None,
            initialization_script: None,
            on_ipc: Arc::new(|_| {}),
            on_page: None,
        }
    }

    fn kind_of(call: &Call) -> &'static str {
        match call {
            Call::Create { .. } => "create",
            Call::Focus { .. } => "focus",
            Call::FocusParent { .. } => "focus_parent",
            Call::ClearBrowsingData => "clear",
            Call::Destroy { .. } => "destroy",
            _ => "other",
        }
    }

    #[test]
    fn views_are_created_laid_out_focused_and_destroyed_in_order() {
        let mock = MockPlatform::default();
        let id = WebViewId(1);
        mock.create(id, ParentWindow::X11(9), config()).unwrap();
        mock.focus(id).unwrap();
        mock.focus_parent(id).unwrap();
        mock.clear_browsing_data().unwrap();
        mock.destroy(id).unwrap();
        let kinds: Vec<&str> = mock.calls().iter().map(kind_of).collect();
        assert_eq!(
            kinds,
            ["create", "focus", "focus_parent", "clear", "destroy"],
            "order"
        );
    }
}
