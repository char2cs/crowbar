use raw_window_handle::RawWindowHandle;
use x11rb::CURRENT_TIME;
use x11rb::NONE;
use x11rb::connection::Connection as _;
use x11rb::errors::{ConnectError, ReplyError};
use x11rb::properties::{WmSizeHints, WmSizeHintsSpecification};
use x11rb::protocol::shape::{ConnectionExt as _, SK, SO};
use x11rb::protocol::xproto::{
    ClientMessageEvent, ClipOrdering, ConfigureWindowAux, ConnectionExt as _, EventMask,
    InputFocus as RevertTo, StackMode,
};
use x11rb::rust_connection::RustConnection;
use x11rb::wrapper::ConnectionExt as _;

use crate::error::Error;
use crate::linux::float::atoms::Atoms;
use crate::linux::float::card::Card;
use crate::linux::float::region::rectangles;

const STATE_ADD: u32 = 1;
const WORDS: u8 = 32;

/// Places, shapes and focuses the windows that float above a shell window, over a connection
/// of its own, given the raw handles of stock GPUI windows.
///
/// A float window covers its owner's content area exactly, so what it paints lines up with
/// the owner. Without a compositor X shows no translucency, so a float window then shows only
/// its cards.
#[derive(Debug)]
pub struct FloatPlacer {
    connection: RustConnection,
    root: u32,
    atoms: Atoms,
}

impl FloatPlacer {
    /// Connects to the X server named by `DISPLAY`.
    ///
    /// # Errors
    /// [`Error::X11Connect`] when no X server answers, [`Error::Float`] when it will not name
    /// the atoms this needs.
    pub fn connect() -> Result<Self, Error> {
        let (connection, screen) = RustConnection::connect(None)?;
        let root = connection
            .setup()
            .roots
            .get(screen)
            .ok_or(ConnectError::InvalidScreen)?
            .root;
        let compositor = format!("_NET_WM_CM_S{screen}");
        let atoms = Atoms {
            compositor: atom(&connection, &compositor)?,
            state: atom(&connection, "_NET_WM_STATE")?,
            skip_taskbar: atom(&connection, "_NET_WM_STATE_SKIP_TASKBAR")?,
            skip_pager: atom(&connection, "_NET_WM_STATE_SKIP_PAGER")?,
        };
        Ok(Self {
            connection,
            root,
            atoms,
        })
    }

    /// Keeps a float window the window manager manages off the taskbar and pager, and asks
    /// that it stays where [`Self::cover`] puts it.
    ///
    /// # Errors
    /// [`Error::ForeignWindow`] for a handle that is not an X window, [`Error::Float`] when the
    /// connection broke.
    pub fn pin(
        &self,
        float: RawWindowHandle,
    ) -> Result<(), Error> {
        let window = xid(float)?;
        let mut hints = WmSizeHints::new();
        hints.position = Some((WmSizeHintsSpecification::UserSpecified, 0, 0));
        hints
            .set_normal_hints(&self.connection, window)
            .map_err(ReplyError::from)?;
        let skip = [
            STATE_ADD,
            self.atoms.skip_taskbar,
            self.atoms.skip_pager,
            0,
            0,
        ];
        let message = ClientMessageEvent::new(WORDS, window, self.atoms.state, skip);
        let reach = EventMask::SUBSTRUCTURE_REDIRECT | EventMask::SUBSTRUCTURE_NOTIFY;
        self.connection
            .send_event(false, self.root, reach, message)
            .map_err(ReplyError::from)?;
        self.sync()
    }

    /// Moves and sizes `float` over the content area of `owner` and raises it.
    ///
    /// # Errors
    /// [`Error::ForeignWindow`] for a handle that is not an X window, [`Error::Float`] when
    /// either window is gone or the connection broke.
    pub fn cover(
        &self,
        float: RawWindowHandle,
        owner: RawWindowHandle,
    ) -> Result<(), Error> {
        let (float, owner) = (xid(float)?, xid(owner)?);
        let origin = self
            .connection
            .translate_coordinates(owner, self.root, 0, 0)?
            .reply()?;
        let size = self.connection.get_geometry(owner)?.reply()?;
        let place = ConfigureWindowAux::new()
            .x(i32::from(origin.dst_x))
            .y(i32::from(origin.dst_y))
            .width(u32::from(size.width))
            .height(u32::from(size.height))
            .stack_mode(StackMode::ABOVE);
        self.connection
            .configure_window(float, &place)
            .map_err(ReplyError::from)?;
        self.sync()
    }

    /// Shows only `seen` of `float` when no compositor runs, all of it otherwise, and lets
    /// the pointer reach it only on `hit`; elsewhere the pointer goes to the window below.
    ///
    /// # Errors
    /// [`Error::ForeignWindow`] for a handle that is not an X window, [`Error::Float`] when the
    /// connection broke.
    pub fn shape(
        &self,
        float: RawWindowHandle,
        seen: &[Card],
        hit: &[Card],
    ) -> Result<(), Error> {
        let window = xid(float)?;
        self.set_bounding(window, seen)?;
        self.set_region(window, SK::INPUT, hit)?;
        self.sync()
    }

    fn set_bounding(
        &self,
        window: u32,
        seen: &[Card],
    ) -> Result<(), Error> {
        if !self.composited()? {
            return self.set_region(window, SK::BOUNDING, seen);
        }
        self.connection
            .shape_mask(SO::SET, SK::BOUNDING, window, 0, 0, NONE)
            .map_err(ReplyError::from)?;
        Ok(())
    }

    /// Gives the keyboard to `float`.
    ///
    /// # Errors
    /// [`Error::ForeignWindow`] for a handle that is not an X window, [`Error::Focus`] when the
    /// window is not viewable.
    pub fn focus(
        &self,
        float: RawWindowHandle,
    ) -> Result<(), Error> {
        let window = xid(float)?;
        self.connection
            .set_input_focus(RevertTo::PARENT, window, CURRENT_TIME)
            .map_err(ReplyError::from)
            .and_then(x11rb::cookie::VoidCookie::check)
            .map_err(|source| Error::Focus { window, source })
    }

    fn composited(&self) -> Result<bool, Error> {
        let owner = self
            .connection
            .get_selection_owner(self.atoms.compositor)?
            .reply()?;
        Ok(owner.owner != NONE)
    }

    fn set_region(
        &self,
        window: u32,
        kind: SK,
        cards: &[Card],
    ) -> Result<(), Error> {
        let rows = rectangles(cards);
        self.connection
            .shape_rectangles(SO::SET, kind, ClipOrdering::UNSORTED, window, 0, 0, &rows)
            .map_err(ReplyError::from)?;
        Ok(())
    }

    // A round trip, not a flush: other connections, GPUI's among them, see the change on return.
    fn sync(&self) -> Result<(), Error> {
        self.connection.sync()?;
        Ok(())
    }
}

fn atom(
    connection: &RustConnection,
    name: &str,
) -> Result<u32, Error> {
    Ok(connection
        .intern_atom(false, name.as_bytes())?
        .reply()?
        .atom)
}

fn xid(handle: RawWindowHandle) -> Result<u32, Error> {
    match handle {
        RawWindowHandle::Xcb(xcb) => Ok(xcb.window.get()),
        RawWindowHandle::Xlib(xlib) => u32::try_from(xlib.window).or(Err(Error::ForeignWindow)),
        _ => Err(Error::ForeignWindow),
    }
}

#[cfg(test)]
mod tests {
    use std::num::NonZeroU32;

    use raw_window_handle::XcbWindowHandle;
    use x11rb::COPY_DEPTH_FROM_PARENT;
    use x11rb::protocol::xproto::{CreateWindowAux, WindowClass};

    use super::*;

    fn window_at(
        connection: &RustConnection,
        root: u32,
        spot: (i16, i16, u16),
    ) -> u32 {
        let (x, y, side) = spot;
        let window = connection.generate_id().unwrap();
        let aux = CreateWindowAux::new().override_redirect(1);
        connection
            .create_window(
                COPY_DEPTH_FROM_PARENT,
                window,
                root,
                x,
                y,
                side,
                side,
                0,
                WindowClass::INPUT_OUTPUT,
                0,
                &aux,
            )
            .unwrap();
        connection.map_window(window).unwrap();
        connection.flush().unwrap();
        window
    }

    fn raw(window: u32) -> RawWindowHandle {
        RawWindowHandle::Xcb(XcbWindowHandle::new(NonZeroU32::new(window).unwrap()))
    }

    fn a_float_window_covers_its_owner(
        connection: &RustConnection,
        root: u32,
    ) {
        let owner = window_at(connection, root, (30, 40, 200));
        let float = window_at(connection, root, (0, 0, 10));
        let placer = FloatPlacer::connect().unwrap();
        placer.cover(raw(float), raw(owner)).unwrap();
        let placed = connection.get_geometry(float).unwrap().reply().unwrap();
        assert_eq!(
            (placed.x, placed.y, placed.width, placed.height),
            (30, 40, 200, 200)
        );
    }

    fn the_pointer_reaches_a_float_window_only_on_its_hit_cards(
        connection: &RustConnection,
        root: u32,
    ) {
        let float = window_at(connection, root, (0, 0, 100));
        let placer = FloatPlacer::connect().unwrap();
        let hit = Card {
            x: 5,
            y: 6,
            width: 20,
            height: 10,
            radius: 0,
        };
        placer.shape(raw(float), &[hit], &[hit]).unwrap();
        let input = connection
            .shape_get_rectangles(float, SK::INPUT)
            .unwrap()
            .reply()
            .unwrap();
        let rects: Vec<_> = input
            .rectangles
            .iter()
            .map(|rect| (rect.x, rect.y, rect.width, rect.height))
            .collect();
        assert_eq!(rects, [(5, 6, 20, 10)]);
        placer.shape(raw(float), &[], &[]).unwrap();
        let input = connection
            .shape_get_rectangles(float, SK::INPUT)
            .unwrap()
            .reply()
            .unwrap();
        assert!(input.rectangles.is_empty(), "nothing takes the pointer");
    }

    #[test]
    #[ignore = "needs display"]
    fn float_windows_are_placed_and_shaped_on_a_display() {
        let (connection, screen) = RustConnection::connect(None).unwrap();
        let root = connection.setup().roots[screen].root;
        a_float_window_covers_its_owner(&connection, root);
        the_pointer_reaches_a_float_window_only_on_its_hit_cards(&connection, root);
    }

    #[test]
    fn a_handle_that_is_not_an_x_window_is_refused() {
        let web = RawWindowHandle::Web(raw_window_handle::WebWindowHandle::new(1));
        assert!(matches!(xid(web), Err(Error::ForeignWindow)));
    }
}
