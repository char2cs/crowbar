//! The requests the shell is still carrying out.

use crate::shell::switcher::icon_target::IconTarget;

/// The requests the shell is still carrying out.
pub trait Requests {
    /// Whether a request that changes the icon of `target` is still running.
    fn icon_busy(
        &self,
        target: &IconTarget,
    ) -> bool;

    /// Whether a request adding `folder` as a space or a repository is still running.
    fn adding(
        &self,
        folder: &str,
    ) -> bool;
}
