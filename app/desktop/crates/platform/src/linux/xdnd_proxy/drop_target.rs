use x11rb::NONE;
use x11rb::errors::ReplyError;
use x11rb::protocol::xproto::{AtomEnum, ConnectionExt as _, TranslateCoordinatesReply};
use x11rb::rust_connection::RustConnection;

const HALF: u32 = 16;
const LOW_HALF: u32 = 0xffff;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct DropTarget {
    pub(crate) window: u32,
    pub(crate) point: u32,
}

impl DropTarget {
    pub(crate) fn under(
        connection: &RustConnection,
        aware: u32,
        ends: (u32, u32),
        point: u32,
    ) -> Result<Option<Self>, ReplyError> {
        let (root, window) = ends;
        let in_window = point_in(connection, root, window, point)?;
        let child = in_window.child;
        if child == NONE || !is_aware(connection, aware, child)? {
            return Ok(None);
        }
        let in_child = point_in(connection, root, child, point)?;
        if in_child.child == NONE {
            return Ok(Some(Self::at(child, &in_window)));
        }
        Ok(Some(Self::at(in_child.child, &in_child)))
    }

    pub(crate) fn at(
        window: u32,
        in_parent: &TranslateCoordinatesReply,
    ) -> Self {
        let x = u32::from(in_parent.dst_x.cast_unsigned());
        let y = u32::from(in_parent.dst_y.cast_unsigned());
        Self {
            window,
            point: (x << HALF) | y,
        }
    }
}

fn point_in(
    connection: &RustConnection,
    root: u32,
    window: u32,
    point: u32,
) -> Result<TranslateCoordinatesReply, ReplyError> {
    let x = u16::try_from(point >> HALF)
        .unwrap_or_default()
        .cast_signed();
    let y = u16::try_from(point & LOW_HALF)
        .unwrap_or_default()
        .cast_signed();
    connection
        .translate_coordinates(root, window, x, y)?
        .reply()
}

fn is_aware(
    connection: &RustConnection,
    aware: u32,
    window: u32,
) -> Result<bool, ReplyError> {
    let reply = connection
        .get_property(false, window, aware, AtomEnum::ATOM, 0, 1)?
        .reply()?;
    Ok(reply.value_len > 0)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_point_packs_x_above_y_as_xdnd_does() {
        let reply = TranslateCoordinatesReply {
            dst_x: 3,
            dst_y: 4,
            ..TranslateCoordinatesReply::default()
        };
        assert_eq!(DropTarget::at(9, &reply).point, (3 << 16) | 4);
    }
}
