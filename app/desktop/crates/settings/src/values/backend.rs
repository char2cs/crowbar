//! The modal's access to setting values.

use crate::values::follow::Follow;
use crate::values::read::Read;
use crate::values::write::Write;

/// Where settings live; the modal reads once, writes on every edit and follows changes.
///
/// The view calls this on the UI thread only: an implementation that talks to the daemon must
/// return quickly and deliver changes from its own task.
pub trait Backend: Read + Write + Follow {}

impl<T: Read + Write + Follow> Backend for T {}
