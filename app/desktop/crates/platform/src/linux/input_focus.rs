use x11rb::CURRENT_TIME;
use x11rb::cookie::VoidCookie;
use x11rb::errors::ReplyError;
use x11rb::protocol::xproto::{ConnectionExt as _, InputFocus as RevertTo};
use x11rb::rust_connection::RustConnection;

use crate::error::Error;

/// The X server's keyboard focus, moved over a connection of its own.
///
/// X sends a key to the focus window, or to the window under the pointer when that one lies
/// inside the focus window. A web view lives in its own child window of the shell's window, so
/// it takes keys wherever the pointer is only while it holds the focus itself.
#[derive(Debug)]
pub struct InputFocus {
    connection: RustConnection,
}

impl InputFocus {
    /// Connects to the X server named by `DISPLAY`.
    ///
    /// # Errors
    /// [`Error::X11Connect`] when no X server answers.
    pub fn connect() -> Result<Self, Error> {
        let (connection, _screen) = RustConnection::connect(None)?;
        Ok(Self { connection })
    }

    /// Gives the keyboard to `window` and waits until the server has done it. When the window
    /// is unmapped later, the focus falls back to its parent.
    ///
    /// # Errors
    /// [`Error::Focus`] when the window is gone or not viewable, or the connection broke.
    pub fn give_to(
        &self,
        window: u32,
    ) -> Result<(), Error> {
        self.connection
            .set_input_focus(RevertTo::PARENT, window, CURRENT_TIME)
            .map_err(ReplyError::from)
            .and_then(VoidCookie::check)
            .map_err(|source| Error::Focus { window, source })
    }
}

#[cfg(test)]
mod tests {
    use x11rb::COPY_DEPTH_FROM_PARENT;
    use x11rb::connection::Connection as _;
    use x11rb::protocol::xproto::{CreateWindowAux, WindowClass};

    use super::*;

    const SIDE: u16 = 10;

    fn window_in(
        connection: &RustConnection,
        parent: u32,
    ) -> u32 {
        let window = connection.generate_id().unwrap();
        connection
            .create_window(
                COPY_DEPTH_FROM_PARENT,
                window,
                parent,
                0,
                0,
                SIDE,
                SIDE,
                0,
                WindowClass::INPUT_OUTPUT,
                0,
                &CreateWindowAux::new().override_redirect(1),
            )
            .unwrap();
        window
    }

    fn holder(connection: &RustConnection) -> u32 {
        connection.get_input_focus().unwrap().reply().unwrap().focus
    }

    #[test]
    #[ignore = "needs display"]
    fn the_keyboard_goes_to_a_viewable_window_and_back_to_its_parent_when_it_hides() {
        let (connection, screen) = RustConnection::connect(None).unwrap();
        let root = connection.setup().roots[screen].root;
        let parent = window_in(&connection, root);
        let child = window_in(&connection, parent);
        let unmapped = window_in(&connection, parent);
        connection.map_window(parent).unwrap();
        connection.map_window(child).unwrap();
        connection.flush().unwrap();
        let focus = InputFocus::connect().unwrap();
        focus.give_to(child).unwrap();
        assert_eq!(holder(&connection), child, "a mapped child takes it");
        connection.unmap_window(child).unwrap();
        assert_eq!(holder(&connection), parent, "it falls back to the parent");
        assert!(
            matches!(focus.give_to(unmapped), Err(Error::Focus { window, .. }) if window == unmapped),
            "an unmapped window is refused"
        );
    }
}
