use crate::backend::desktop::caret_blink::CaretReport;
use crate::backend::desktop::color_scheme::ColorScheme;
use crate::backend::desktop::desktop_settings::DesktopSettings;
use crate::backend::mock::call::Call;
use crate::backend::mock::mock_platform::MockPlatform;
use crate::backend::view::browsing_data::BrowsingData;
use crate::backend::view::view_cutout::ViewCutout;
use crate::backend::view::view_focus::ViewFocus;
use crate::backend::view::view_layout::ViewLayout;
use crate::backend::view::view_lifecycle::ViewLifecycle;
use crate::error::{Error, Result};
use crate::view::Bounds;
use crate::view::Color;
use crate::view::Config;
use crate::view::Id;
use crate::view::ParentWindow;
use crate::view::Visibility;

impl ViewLifecycle for MockPlatform {
    fn create(
        &self,
        id: Id,
        parent: ParentWindow,
        config: Config,
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
        id: Id,
    ) -> Result<()> {
        self.live
            .borrow_mut()
            .remove(&id)
            .ok_or(Error::UnknownWebView(id))?;
        self.record(Call::Destroy { id });
        Ok(())
    }

    fn settle(&self) -> Result<()> {
        self.record(Call::Settle);
        Ok(())
    }
}

impl ViewLayout for MockPlatform {
    fn set_bounds(
        &self,
        id: Id,
        bounds: Bounds,
    ) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::SetBounds { id, bounds });
        Ok(())
    }

    fn set_visibility(
        &self,
        id: Id,
        visibility: Visibility,
    ) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::SetVisibility { id, visibility });
        Ok(())
    }

    fn set_background(
        &self,
        id: Id,
        color: Color,
    ) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::SetBackground { id, color });
        Ok(())
    }
}

impl ViewCutout for MockPlatform {
    fn set_cutout(
        &self,
        id: Id,
        hole: Option<Bounds>,
    ) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::SetCutout { id, hole });
        Ok(())
    }
}

impl ViewFocus for MockPlatform {
    fn focus(
        &self,
        id: Id,
    ) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::Focus { id });
        Ok(())
    }

    fn focus_parent(
        &self,
        id: Id,
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

impl DesktopSettings for MockPlatform {
    fn follow_caret(
        &self,
        _report: CaretReport,
    ) -> Result<()> {
        self.record(Call::FollowCaret);
        Ok(())
    }

    fn draw_widgets_in(
        &self,
        scheme: ColorScheme,
    ) -> Result<()> {
        self.record(Call::DrawWidgetsIn { scheme });
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::view::Kind;

    fn config() -> Config {
        let bounds = Bounds {
            x: 0.0,
            y: 0.0,
            width: 10.0,
            height: 10.0,
        };
        Config {
            url: "crowbar://local/".into(),
            bounds,
            kind: Kind::Core,
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
            Call::FollowCaret => "follow_caret",
            Call::Destroy { .. } => "destroy",
            _ => "other",
        }
    }

    #[test]
    fn views_are_created_laid_out_focused_and_destroyed_in_order() {
        let mock = MockPlatform::default();
        let id = Id(1);
        mock.create(id, ParentWindow::X11(9), config()).unwrap();
        mock.focus(id).unwrap();
        mock.focus_parent(id).unwrap();
        mock.clear_browsing_data().unwrap();
        mock.follow_caret(Arc::new(|_| ())).unwrap();
        mock.destroy(id).unwrap();
        let kinds: Vec<&str> = mock.calls().iter().map(kind_of).collect();
        assert_eq!(
            kinds,
            [
                "create",
                "focus",
                "focus_parent",
                "clear",
                "follow_caret",
                "destroy"
            ],
            "order"
        );
    }
}
