//! The write side of a settings backend.

use crate::values::error::Error;
use crate::values::key::Key;
use crate::values::value::Value;

/// What the modal changes: values and backend actions.
pub trait SettingsWrite {
    /// Stores `value` under `key`.
    ///
    /// # Errors
    /// [`Error::Rejected`] when the backend refuses the value, [`Error::Unavailable`] when it cannot be reached.
    fn set(
        &self,
        key: &Key,
        value: Value,
    ) -> Result<(), Error>;

    /// Runs the backend action `id`, such as `browser.clearData`.
    ///
    /// # Errors
    /// [`Error::Unavailable`] when the action cannot run.
    fn invoke(
        &self,
        id: &str,
    ) -> Result<(), Error>;
}
