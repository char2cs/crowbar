//! How a settings backend reports changes made elsewhere.

use std::rc::Rc;

use crate::change::Change;
use crate::subscription::Subscription;

/// Callback receiving every [`Change`] a backend reports.
pub type Listener = Rc<dyn Fn(&Change)>;

/// How the modal hears about changes made elsewhere.
pub trait SettingsFollow {
    /// Reports every change to `listener` until the subscription is dropped.
    fn subscribe(
        &self,
        listener: Listener,
    ) -> Subscription;
}
