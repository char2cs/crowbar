//! The icon requests the shell is still carrying out.

use crate::shell::switcher::icon_target::IconTarget;

/// The icon requests the shell is still carrying out.
pub trait IconRequests {
    /// Whether a request that changes the icon of `target` is still running.
    fn icon_busy(
        &self,
        target: &IconTarget,
    ) -> bool;
}
