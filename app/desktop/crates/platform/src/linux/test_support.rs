use x11rb::COPY_DEPTH_FROM_PARENT;
use x11rb::connection::Connection as _;
use x11rb::protocol::shape::{ConnectionExt as _, SK};
use x11rb::protocol::xproto::{ConnectionExt as _, CreateWindowAux, WindowClass};
use x11rb::rust_connection::RustConnection;

pub(crate) type Geometry = (i16, i16, u16, u16);

pub(crate) fn window_in(
    connection: &RustConnection,
    parent: u32,
    geometry: Geometry,
) -> u32 {
    let (x, y, width, height) = geometry;
    let window = connection.generate_id().unwrap();
    let aux = CreateWindowAux::new().override_redirect(1);
    connection
        .create_window(
            COPY_DEPTH_FROM_PARENT,
            window,
            parent,
            x,
            y,
            width,
            height,
            0,
            WindowClass::INPUT_OUTPUT,
            0,
            &aux,
        )
        .unwrap();
    window
}

pub(crate) fn mapped_in(
    connection: &RustConnection,
    parent: u32,
    geometry: Geometry,
) -> u32 {
    let window = window_in(connection, parent, geometry);
    connection.map_window(window).unwrap();
    connection.flush().unwrap();
    window
}

pub(crate) fn input_rectangles(
    connection: &RustConnection,
    window: u32,
) -> Vec<Geometry> {
    let input = connection
        .shape_get_rectangles(window, SK::INPUT)
        .unwrap()
        .reply()
        .unwrap();
    input
        .rectangles
        .iter()
        .map(|rect| (rect.x, rect.y, rect.width, rect.height))
        .collect()
}
