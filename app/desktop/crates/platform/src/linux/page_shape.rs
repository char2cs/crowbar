//! Cuts a hole and rounded corners out of a web view's window, or all of it, so the shell window under it shows
//! there and takes the pointer, as for the handle a split pane hangs from its top.

mod page_cut;
mod rounded;

use x11rb::NONE;
use x11rb::connection::Connection as _;
use x11rb::errors::ReplyError;
use x11rb::protocol::shape::{ConnectionExt as _, SK, SO};
use x11rb::protocol::xproto::{ClipOrdering, ConnectionExt as _, QueryTreeReply, Rectangle};
use x11rb::rust_connection::RustConnection;

use crate::error::Error;
use crate::linux::float::region::{inset, rectangles};

pub use page_cut::PageCut;
pub use rounded::Rounded;

const WHOLE: Rectangle = Rectangle {
    x: 0,
    y: 0,
    width: u16::MAX,
    height: u16::MAX,
};

fn notches(round: Rounded) -> impl Iterator<Item = Rectangle> {
    let (right, bottom) = (Some(round.width), Some(round.height));
    let far = [(None, None), (right, None), (right, bottom), (None, bottom)];
    far.into_iter()
        .zip(round.radii)
        .flat_map(|(far, radius)| notch(far, radius))
}

fn notch(
    (right, bottom): (Option<u16>, Option<u16>),
    radius: u16,
) -> impl Iterator<Item = Rectangle> {
    let rows = (0..radius).map(move |row| outside((right, bottom), (radius, row)));
    rows.filter(|row| row.width > 0)
}

fn outside(
    (right, bottom): (Option<u16>, Option<u16>),
    (radius, row): (u16, u16),
) -> Rectangle {
    let width = inset(radius, row);
    let x = right.map_or(0, |edge| edge.saturating_sub(width));
    let y = bottom.map_or(row, |edge| edge.saturating_sub(row + 1));
    Rectangle {
        x: x.cast_signed(),
        y: y.cast_signed(),
        width,
        height: 1,
    }
}

/// The shape of the windows web views draw in, set over a connection of its own.
#[derive(Debug)]
pub struct PageShape {
    connection: RustConnection,
}

impl PageShape {
    /// Connects to the X server named by `DISPLAY`.
    ///
    /// # Errors
    /// [`Error::X11Connect`] when no X server answers.
    pub fn connect() -> Result<Self, Error> {
        let (connection, _screen) = RustConnection::connect(None)?;
        Ok(Self { connection })
    }

    /// Cuts `cut` out of the child of `owner` that holds the window `page`, for drawing and for
    /// the pointer alike; `None` makes it whole again. The cut holds however the window is
    /// resized, hidden or shown later.
    ///
    /// # Errors
    /// [`Error::Cutout`] when the window is gone or the connection broke.
    pub fn cut(
        &self,
        (page, owner): (u32, u32),
        cut: Option<PageCut>,
    ) -> Result<(), Error> {
        let window = self.child_of(owner, page)?;
        let shape = |kind| self.cut_kind(window, kind, cut);
        shape(SK::BOUNDING)
            .and_then(|()| shape(SK::INPUT))
            .map_err(|source| Error::Cutout { window, source })?;
        self.connection
            .flush()
            .map_err(ReplyError::from)
            .map_err(|source| Error::Cutout { window, source })
    }

    fn cut_kind(
        &self,
        window: u32,
        kind: SK,
        cut: Option<PageCut>,
    ) -> Result<(), ReplyError> {
        let unsorted = ClipOrdering::UNSORTED;
        let Some(cut) = cut else {
            self.connection
                .shape_mask(SO::SET, kind, window, 0, 0, NONE)?;
            return Ok(());
        };
        let PageCut::Trimmed { hole, round } = cut else {
            self.connection
                .shape_rectangles(SO::SET, kind, unsorted, window, 0, 0, &[])?;
            return Ok(());
        };
        let mut rows = rectangles(hole.as_slice());
        rows.extend(round.into_iter().flat_map(notches));
        self.connection
            .shape_rectangles(SO::SET, kind, unsorted, window, 0, 0, &[WHOLE])?;
        self.connection
            .shape_rectangles(SO::SUBTRACT, kind, unsorted, window, 0, 0, &rows)?;
        Ok(())
    }

    fn child_of(
        &self,
        owner: u32,
        page: u32,
    ) -> Result<u32, Error> {
        let mut window = page;
        let mut tree = self.tree_of(window)?;
        let at_the_top = |tree: &QueryTreeReply| {
            tree.parent == owner || tree.parent == tree.root || tree.parent == NONE
        };
        while !at_the_top(&tree) {
            window = tree.parent;
            tree = self.tree_of(window)?;
        }
        Ok(window)
    }

    fn tree_of(
        &self,
        window: u32,
    ) -> Result<QueryTreeReply, Error> {
        self.connection
            .query_tree(window)
            .map_err(ReplyError::from)
            .and_then(x11rb::cookie::Cookie::reply)
            .map_err(|source| Error::Cutout { window, source })
    }
}

#[cfg(test)]
mod tests {
    use x11rb::COPY_DEPTH_FROM_PARENT;
    use x11rb::protocol::xproto::{CreateWindowAux, WindowClass};

    use super::*;
    use crate::linux::float::Card;

    fn child(
        connection: &RustConnection,
        parent: u32,
    ) -> u32 {
        let window = connection.generate_id().unwrap();
        let aux = CreateWindowAux::new().override_redirect(1);
        connection
            .create_window(
                COPY_DEPTH_FROM_PARENT,
                window,
                parent,
                0,
                0,
                200,
                100,
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

    fn input(
        connection: &RustConnection,
        window: u32,
    ) -> Vec<(i16, i16, u16, u16)> {
        let input = connection
            .shape_get_rectangles(window, SK::INPUT)
            .unwrap()
            .reply()
            .unwrap();
        // PLATFORM: linux X hands back the region as set; only its part inside the window counts.
        input
            .rectangles
            .iter()
            .map(|rect| {
                let width = rect
                    .width
                    .min(200u16.saturating_sub(rect.x.cast_unsigned()));
                let height = rect
                    .height
                    .min(100u16.saturating_sub(rect.y.cast_unsigned()));
                (rect.x, rect.y, width, height)
            })
            .filter(|&(_, _, width, height)| width > 0 && height > 0)
            .collect()
    }

    #[test]
    fn only_the_pixels_outside_a_rounded_corner_are_notched() {
        let page = Rounded {
            width: 100,
            height: 50,
            radii: [4, 0, 4, 0],
        };
        let notched: Vec<(i16, i16, u16, u16)> = notches(page)
            .map(|rect| (rect.x, rect.y, rect.width, rect.height))
            .collect();
        assert!(notched.contains(&(0, 0, 2, 1)), "top left: {notched:?}");
        let (x, y, width, _) = notched[notched.len() - 1];
        assert!(
            x > 90 && x + width.cast_signed() == 100 && y > 45,
            "bottom right: {notched:?}"
        );
        let square = |&(x, y, _, _): &(i16, i16, u16, u16)| (x > 50) != (y > 25);
        assert!(!notched.iter().any(square), "square corners: {notched:?}");
    }

    /// A window, its child and its grandchild: the shell, wry's holder and the page.
    fn nested(
        connection: &RustConnection,
        root: u32,
    ) -> [u32; 3] {
        let owner = child(connection, root);
        let holder = child(connection, owner);
        [owner, holder, child(connection, holder)]
    }

    /// X serves each connection on its own, so a cut is read only once the server has done it.
    fn settled(shape: &PageShape) {
        shape.connection.get_input_focus().unwrap().reply().unwrap();
    }

    fn rounds_the_bottom_corners(
        shape: &PageShape,
        cut: (u32, u32),
        (connection, holder): (&RustConnection, u32),
    ) {
        let round = Rounded {
            width: 200,
            height: 100,
            radii: [0, 0, 9, 9],
        };
        let trimmed = PageCut::Trimmed {
            hole: None,
            round: Some(round),
        };
        shape.cut(cut, Some(trimmed)).unwrap();
        settled(shape);
        let rounded = input(connection, holder);
        let ends = (rounded.first(), rounded.last());
        assert_eq!(ends, (Some(&(0, 0, 200, 94)), Some(&(6, 99, 188, 1))));
    }

    #[test]
    #[ignore = "needs display"]
    fn the_hole_is_cut_from_the_owner_s_child_and_filled_again_on_a_display() {
        let (connection, screen) = RustConnection::connect(None).unwrap();
        let root = connection.setup().roots[screen].root;
        let [owner, holder, page] = nested(&connection, root);
        let shape = PageShape::connect().unwrap();
        let hole = Card {
            x: 60,
            y: 0,
            width: 70,
            height: 20,
            radius: 0,
        };
        let holed = PageCut::Trimmed {
            hole: Some(hole),
            round: None,
        };
        shape.cut((page, owner), Some(holed)).unwrap();
        settled(&shape);
        let cut = input(&connection, holder);
        assert!(!cut.contains(&(0, 0, 200, 100)), "{cut:?}");
        assert!(cut.contains(&(0, 20, 200, 80)), "below the hole: {cut:?}");
        rounds_the_bottom_corners(&shape, (page, owner), (&connection, holder));
        shape.cut((page, owner), Some(PageCut::Everything)).unwrap();
        settled(&shape);
        assert_eq!(input(&connection, holder), [], "all cut away");
        shape.cut((page, owner), None).unwrap();
        settled(&shape);
        assert_eq!(
            input(&connection, holder),
            [(0, 0, 200, 100)],
            "whole again"
        );
    }
}
