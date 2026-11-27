pub(super) mod icon_target;

use crate::shell::switcher::requests::icon_target::IconTarget;

pub trait Requests {
    fn icon_busy(
        &self,
        target: &IconTarget,
    ) -> bool;

    fn adding(
        &self,
        folder: &str,
    ) -> bool;
}
