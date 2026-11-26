use gtk::glib::Cast as _;
use x11rb::protocol::xproto::{AtomEnum, ConnectionExt as _};

pub(crate) fn xid(window: &gtk::gdk::Window) -> u32 {
    window
        .clone()
        .downcast::<gdkx11::X11Window>()
        .ok()
        .and_then(|window| u32::try_from(window.xid()).ok())
        .expect("an X11 window")
}

pub(crate) fn transient_for(window: u32) -> Option<u32> {
    let (connection, _) = x11rb::connect(None).expect("the X display");
    let reply = connection
        .get_property(
            false,
            window,
            AtomEnum::WM_TRANSIENT_FOR,
            AtomEnum::WINDOW,
            0,
            1,
        )
        .expect("a request")
        .reply()
        .expect("a reply");
    reply.value32().and_then(|mut values| values.next())
}
