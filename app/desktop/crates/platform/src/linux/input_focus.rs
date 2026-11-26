use std::os::fd::{AsRawFd, RawFd};

use x11rb::CURRENT_TIME;
use x11rb::connection::Connection as _;
use x11rb::cookie::VoidCookie;
use x11rb::errors::{ConnectionError, ReplyError};
use x11rb::protocol::xproto::{
    ChangeWindowAttributesAux, ConnectionExt as _, EventMask, InputFocus as RevertTo,
};
use x11rb::rust_connection::RustConnection;

use crate::error::Error;
use crate::linux::focus_change::FocusChange;

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

    /// Reports through [`Self::changes`] when `window` gains or loses the keyboard, whoever
    /// moves it: this client, the window manager or another application.
    ///
    /// # Errors
    /// [`Error::Follow`] when the window is gone or the connection broke.
    pub fn follow(
        &self,
        window: u32,
    ) -> Result<(), Error> {
        let events = ChangeWindowAttributesAux::new().event_mask(EventMask::FOCUS_CHANGE);
        self.connection
            .change_window_attributes(window, &events)
            .map_err(ReplyError::from)
            .and_then(VoidCookie::check)
            .map_err(|source| Error::Follow { window, source })
    }

    /// The focus changes of followed windows that have arrived, oldest first, without waiting.
    /// Read them when [`AsRawFd::as_raw_fd`] turns readable and after [`Self::give_to`], which
    /// queues the changes it causes.
    #[must_use]
    pub fn changes(&self) -> Vec<FocusChange> {
        std::iter::from_fn(|| {
            self.connection
                .poll_for_event()
                .inspect_err(|error| tracing::warn!(%error, "the X focus connection broke"))
                .ok()
                .flatten()
        })
        .filter_map(|event| FocusChange::of(&event))
        .collect()
    }

    /// Waits for the next focus change of a followed window, the way a thread of its own
    /// follows the keyboard.
    ///
    /// # Errors
    /// [`Error::FocusWait`] when the connection broke.
    pub fn next_change(&self) -> Result<FocusChange, Error> {
        std::iter::repeat_with(|| self.connection.wait_for_event())
            .find_map(|event| event.map(|event| FocusChange::of(&event)).transpose())
            .unwrap_or(Err(ConnectionError::UnknownError))
            .map_err(Error::FocusWait)
    }
}

impl AsRawFd for InputFocus {
    fn as_raw_fd(&self) -> RawFd {
        self.connection.stream().as_raw_fd()
    }
}

#[cfg(test)]
mod tests {
    use x11rb::COPY_DEPTH_FROM_PARENT;
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

    fn the_keyboard_goes_to_a_viewable_window_and_back_to_its_parent_when_it_hides(
        connection: &RustConnection,
        root: u32,
    ) {
        let parent = window_in(connection, root);
        let child = window_in(connection, parent);
        let unmapped = window_in(connection, parent);
        connection.map_window(parent).unwrap();
        connection.map_window(child).unwrap();
        connection.flush().unwrap();
        let focus = InputFocus::connect().unwrap();
        focus.give_to(child).unwrap();
        assert_eq!(holder(connection), child, "a mapped child takes it");
        connection.unmap_window(child).unwrap();
        assert_eq!(holder(connection), parent, "it falls back to the parent");
        assert!(
            matches!(focus.give_to(unmapped), Err(Error::Focus { window, .. }) if window == unmapped),
            "an unmapped window is refused"
        );
    }

    fn a_followed_window_reports_the_keyboard_coming_and_going(
        connection: &RustConnection,
        root: u32,
    ) {
        let parent = window_in(connection, root);
        let child = window_in(connection, parent);
        let elsewhere = window_in(connection, root);
        for window in [parent, child, elsewhere] {
            connection.map_window(window).unwrap();
        }
        connection.flush().unwrap();
        let focus = InputFocus::connect().unwrap();
        focus.give_to(parent).unwrap();
        focus.follow(child).unwrap();
        let (gained, lost) = (FocusChange::Gained(child), FocusChange::Lost(child));
        focus.give_to(child).unwrap();
        assert_eq!(focus.changes(), [gained], "given to it");
        focus.give_to(parent).unwrap();
        assert_eq!(focus.changes(), [lost], "back to its parent");
        focus.give_to(child).unwrap();
        connection
            .set_input_focus(RevertTo::PARENT, elsewhere, CURRENT_TIME)
            .unwrap()
            .check()
            .unwrap();
        focus.give_to(child).unwrap();
        assert_eq!(
            focus.changes(),
            [gained, lost, gained],
            "taken by another client and given back"
        );
        focus.give_to(parent).unwrap();
        assert_eq!(focus.next_change().unwrap(), lost, "waited for");
    }

    #[test]
    #[ignore = "needs display"]
    fn the_keyboard_moves_between_windows_and_its_moves_are_heard() {
        let (connection, screen) = RustConnection::connect(None).unwrap();
        let root = connection.setup().roots[screen].root;
        the_keyboard_goes_to_a_viewable_window_and_back_to_its_parent_when_it_hides(
            &connection,
            root,
        );
        a_followed_window_reports_the_keyboard_coming_and_going(&connection, root);
    }
}
