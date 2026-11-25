use crate::error::Result;
use crate::view::Id;

/// Which side holds keyboard focus: the page or the parent window.
pub trait ViewFocus {
    /// Gives keyboard focus to the page: the window system then sends it the keys wherever the
    /// pointer is, until the parent window or another page takes them.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn focus(
        &self,
        id: Id,
    ) -> Result<()>;

    /// Gives keyboard focus back to the parent window the view was created in.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn focus_parent(
        &self,
        id: Id,
    ) -> Result<()>;
}
