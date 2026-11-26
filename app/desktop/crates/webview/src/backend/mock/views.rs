use crate::backend::mock::call::Call;
use crate::backend::mock::mock_platform::MockPlatform;
use crate::backend::view::page_control::PageControl;
use crate::backend::view::view_capture::ViewCapture;
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
use crate::view::Snapshot;
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
    use crate::test_support::view_fixtures::config;
    use crate::view::Kind;

    fn kind_of(call: &Call) -> &'static str {
        match call {
            Call::Create { .. } => "create",
            Call::SetVisibility { .. } => "visibility",
            Call::Focus { .. } => "focus",
            Call::FocusParent { .. } => "focus_parent",
            Call::Eval { .. } => "eval",
            Call::Navigate { .. } => "navigate",
            Call::Snapshot { .. } => "snapshot",
            Call::Destroy { .. } => "destroy",
            _ => "other",
        }
    }

    #[test]
    fn the_life_of_a_view_is_recorded_in_order() {
        let mock = MockPlatform::default();
        let id = Id(1);
        mock.create(id, ParentWindow::X11(9), config(Kind::Core))
            .unwrap();
        mock.set_visibility(id, Visibility::Hidden).unwrap();
        mock.focus(id).unwrap();
        mock.focus_parent(id).unwrap();
        mock.eval(id, "1").unwrap();
        mock.navigate(id, "x").unwrap();
        assert!(matches!(mock.snapshot(id), Ok(None)), "no pixels");
        mock.destroy(id).unwrap();
        let kinds: Vec<&str> = mock.calls().iter().map(kind_of).collect();
        let want = [
            "create",
            "visibility",
            "focus",
            "focus_parent",
            "eval",
            "navigate",
            "snapshot",
            "destroy",
        ];
        assert_eq!(kinds, want, "order");
        assert!(mock.live_views().is_empty(), "nothing alive");
    }

    type Operation = fn(&MockPlatform, Id) -> Result<()>;

    const CASES: &[(&str, Operation)] = &[
        ("eval", |mock, id| mock.eval(id, "1")),
        ("navigate", |mock, id| mock.navigate(id, "x")),
        ("bounds", |mock, id| {
            mock.set_bounds(id, config(Kind::Core).bounds)
        }),
        ("destroy", |mock, id| mock.destroy(id)),
        ("ipc", |mock, id| mock.simulate_ipc(id, "x")),
    ];

    #[test]
    fn unknown_views_are_errors_and_record_nothing() {
        let mock = MockPlatform::default();
        for (name, operation) in CASES {
            assert!(
                matches!(operation(&mock, Id(5)), Err(Error::UnknownWebView(_))),
                "{name}"
            );
        }
        assert!(mock.calls().is_empty(), "nothing recorded");
    }
}
