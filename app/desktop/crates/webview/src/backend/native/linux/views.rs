use std::sync::mpsc;
use std::time::Duration;

use crate::backend::browsing_data::BrowsingData;
use crate::backend::native::linux::bounded_wait::wait_for;
use crate::backend::native::linux::create_reply::CreateReply;
use crate::backend::native::linux::gtk_state::GtkState;
use crate::backend::native::linux::linux_platform::{LinuxPlatform, log_failure};
use crate::backend::view_focus::ViewFocus;
use crate::backend::view_layout::ViewLayout;
use crate::backend::view_lifecycle::ViewLifecycle;
use crate::error::Result;
use crate::view::Bounds;
use crate::view::Color;
use crate::view::ParentWindow;
use crate::view::Visibility;
use crate::view::WebViewConfig;
use crate::view::WebViewId;

const CREATE_PATIENCE: Duration = Duration::from_secs(5);

fn settle_create(
    state: &mut GtkState,
    reply: CreateReply,
    created: Result<()>,
) {
    let Some(abandoned) = reply.send(created) else {
        return;
    };
    log_failure("destroy", state.destroy(abandoned));
}

impl ViewLifecycle for LinuxPlatform {
    fn create(
        &self,
        id: WebViewId,
        parent: ParentWindow,
        config: WebViewConfig,
    ) -> Result<()> {
        let (reply, answer) = mpsc::channel();
        self.post(move |state| {
            let created = state.create(id, parent, &config);
            settle_create(state, CreateReply { reply, id }, created);
        })?;
        wait_for(&answer, CREATE_PATIENCE)
    }

    fn destroy(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        self.post_logged("destroy", move |state| state.destroy(id))
    }
}

impl ViewLayout for LinuxPlatform {
    fn set_bounds(
        &self,
        id: WebViewId,
        bounds: Bounds,
    ) -> Result<()> {
        self.post_logged("set_bounds", move |state| state.set_bounds(id, bounds))
    }

    fn set_visibility(
        &self,
        id: WebViewId,
        visibility: Visibility,
    ) -> Result<()> {
        self.post_logged("set_visibility", move |state| {
            state.set_visibility(id, visibility)
        })
    }

    fn set_background(
        &self,
        id: WebViewId,
        color: Color,
    ) -> Result<()> {
        self.post_logged("set_background", move |state| {
            state.set_background(id, color)
        })
    }
}

impl ViewFocus for LinuxPlatform {
    fn focus(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        self.post_logged("focus", move |state| state.focus(id))
    }

    fn focus_parent(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        self.post_logged("focus_parent", move |state| state.focus_parent(id))
    }
}

impl BrowsingData for LinuxPlatform {
    fn clear_browsing_data(&self) -> Result<()> {
        self.post_logged("clear_browsing_data", GtkState::clear_browsing_data)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn requests_for_unknown_views_are_posted_not_answered_with_errors() {
        let (jobs, _inbox) = async_channel::unbounded();
        let platform = LinuxPlatform { jobs, thread: None };
        let id = WebViewId(9);
        assert!(
            platform.set_visibility(id, Visibility::Hidden).is_ok(),
            "visibility"
        );
        assert!(platform.focus(id).is_ok(), "focus");
        assert!(platform.destroy(id).is_ok(), "destroy");
    }
}
