use raw_window_handle::RawWindowHandle;
use x11rb::NONE;
use x11rb::connection::Connection as _;
use x11rb::errors::{ConnectError, ReplyError};
use x11rb::protocol::shape::{ConnectionExt as _, SK, SO};
use x11rb::protocol::xproto::{
    ClipOrdering, ConfigureWindowAux, ConnectionExt as _, EXPOSE_EVENT, EventMask, ExposeEvent,
    StackMode,
};
use x11rb::rust_connection::RustConnection;

use crate::error::Error;
use crate::linux::float::card::Card;
use crate::linux::float::region::rectangles;

#[derive(Debug)]
pub struct FloatPlacer {
    connection: RustConnection,
    root: u32,
    compositor: u32,
}

impl FloatPlacer {
    pub fn connect() -> Result<Self, Error> {
        let (connection, screen) = RustConnection::connect(None)?;
        let root = connection
            .setup()
            .roots
            .get(screen)
            .ok_or(ConnectError::InvalidScreen)?
            .root;
        let compositor = atom(&connection, &format!("_NET_WM_CM_S{screen}"))?;
        Ok(Self {
            connection,
            root,
            compositor,
        })
    }

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
        self.flush()
    }

    pub fn shape(
        &self,
        float: RawWindowHandle,
        seen: &[Card],
        hit: &[Card],
    ) -> Result<(), Error> {
        let window = xid(float)?;
        self.set_bounding(window, seen)?;
        self.set_region(window, SK::INPUT, hit)?;
        self.flush()
    }

    pub fn wake(
        &self,
        float: RawWindowHandle,
    ) -> Result<(), Error> {
        let window = xid(float)?;
        let exposed = ExposeEvent {
            response_type: EXPOSE_EVENT,
            sequence: 0,
            window,
            x: 0,
            y: 0,
            width: 0,
            height: 0,
            count: 0,
        };
        self.connection
            .send_event(false, window, EventMask::EXPOSURE, exposed)
            .map_err(ReplyError::from)?;
        self.flush()
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

    fn composited(&self) -> Result<bool, Error> {
        let owner = self
            .connection
            .get_selection_owner(self.compositor)?
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

    fn flush(&self) -> Result<(), Error> {
        self.connection.flush().map_err(ReplyError::from)?;
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
    use x11rb::protocol::Event;
    use x11rb::protocol::xproto::{ChangeWindowAttributesAux, CreateWindowAux, WindowClass};
    use x11rb::wrapper::ConnectionExt as _;

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
        placer.connection.sync().unwrap();
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
        placer.connection.sync().unwrap();
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
        placer.connection.sync().unwrap();
        let input = connection
            .shape_get_rectangles(float, SK::INPUT)
            .unwrap()
            .reply()
            .unwrap();
        assert!(input.rectangles.is_empty(), "nothing takes the pointer");
    }

    fn a_float_window_cut_away_whole_is_still_exposed_when_woken(
        connection: &RustConnection,
        root: u32,
    ) {
        let float = window_at(connection, root, (0, 0, 100));
        let listen = ChangeWindowAttributesAux::new().event_mask(EventMask::EXPOSURE);
        connection.change_window_attributes(float, &listen).unwrap();
        let placer = FloatPlacer::connect().unwrap();
        placer.shape(raw(float), &[], &[]).unwrap();
        placer.connection.sync().unwrap();
        connection.sync().unwrap();
        while connection.poll_for_event().unwrap().is_some() {}
        placer.wake(raw(float)).unwrap();
        let heard = connection.wait_for_event().unwrap();
        assert!(matches!(heard, Event::Expose(exposed) if exposed.window == float));
    }

    #[test]
    #[ignore = "needs display"]
    fn float_windows_are_placed_and_shaped_on_a_display() {
        let (connection, screen) = RustConnection::connect(None).unwrap();
        let root = connection.setup().roots[screen].root;
        a_float_window_covers_its_owner(&connection, root);
        the_pointer_reaches_a_float_window_only_on_its_hit_cards(&connection, root);
        a_float_window_cut_away_whole_is_still_exposed_when_woken(&connection, root);
    }

    #[test]
    fn a_handle_that_is_not_an_x_window_is_refused() {
        let web = RawWindowHandle::Web(raw_window_handle::WebWindowHandle::new(1));
        assert!(matches!(xid(web), Err(Error::ForeignWindow)));
    }
}
