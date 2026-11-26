//! Cuts a hole out of a web view's window, or all of it, so the shell window under it shows
//! there and takes the pointer, as for the handle a split pane hangs from its top.

mod page_cut;

use x11rb::NONE;
use x11rb::connection::Connection as _;
use x11rb::errors::ReplyError;
use x11rb::protocol::shape::{ConnectionExt as _, SK, SO};
use x11rb::protocol::xproto::{ClipOrdering, ConnectionExt as _, QueryTreeReply, Rectangle};
use x11rb::rust_connection::RustConnection;

use crate::error::Error;
use crate::linux::float::region::rectangles;

pub use page_cut::PageCut;

const WHOLE: Rectangle = Rectangle {
    x: 0,
    y: 0,
    width: u16::MAX,
    height: u16::MAX,
};

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
        let PageCut::Hole(hole) = cut else {
            self.connection
                .shape_rectangles(SO::SET, kind, unsorted, window, 0, 0, &[])?;
            return Ok(());
        };
        let rows = rectangles(&[hole]);
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
    use super::*;
    use crate::linux::float::Card;
    use crate::linux::test_support::{Geometry, input_rectangles as input, mapped_in};

    const AREA: Geometry = (0, 0, 200, 100);

    #[test]
    #[ignore = "needs display"]
    fn the_hole_is_cut_from_the_owner_s_child_and_filled_again_on_a_display() {
        let (connection, screen) = RustConnection::connect(None).unwrap();
        let root = connection.setup().roots[screen].root;
        let owner = mapped_in(&connection, root, AREA);
        let holder = mapped_in(&connection, owner, AREA);
        let page = mapped_in(&connection, holder, AREA);
        let shape = PageShape::connect().unwrap();
        let hole = Card {
            x: 60,
            y: 0,
            width: 70,
            height: 20,
            radius: 0,
        };
        shape.cut((page, owner), Some(PageCut::Hole(hole))).unwrap();
        let cut = input(&connection, holder);
        assert!(!cut.contains(&(0, 0, 200, 100)), "{cut:?}");
        assert!(cut.contains(&(0, 20, 200, 80)), "below the hole: {cut:?}");
        shape.cut((page, owner), Some(PageCut::Everything)).unwrap();
        assert_eq!(input(&connection, holder), [], "all cut away");
        shape.cut((page, owner), None).unwrap();
        assert_eq!(
            input(&connection, holder),
            [(0, 0, 200, 100)],
            "whole again"
        );
    }
}
