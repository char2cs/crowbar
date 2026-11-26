//! A page selects keys only while the X server says it holds the keyboard, whoever moved it: the
//! shell's own overlay hands the keyboard to the window, not through the page's host.

use x11rb::CURRENT_TIME;
use x11rb::NONE;
use x11rb::connection::Connection as _;
use x11rb::protocol::xproto::{
    ChangeWindowAttributesAux, ConnectionExt as _, EventMask, InputFocus, Screen,
};
use x11rb::rust_connection::RustConnection;

use webview::Visibility;

use super::{INSIDE, KEY_WAIT, KEYCODE_A, TestResult, keys_reaching, page_in, type_key, wait_for};

pub(super) fn the_parent_takes_keys_once_another_client_gives_it_the_keyboard(
    connection: &RustConnection,
    screen: &Screen,
    host: &webview::Host,
    parent: u32,
) -> TestResult {
    let (view, messages) = page_in(host, parent, Visibility::Shown)?;
    let listening = EventMask::STRUCTURE_NOTIFY | EventMask::KEY_PRESS;
    connection.change_window_attributes(
        parent,
        &ChangeWindowAttributesAux::new().event_mask(listening),
    )?;
    connection.warp_pointer(NONE, parent, 0, 0, 0, 0, INSIDE, INSIDE)?;
    view.focus()?;
    wait_for(&messages, "has focus true", KEY_WAIT)?;
    connection.set_input_focus(InputFocus::PARENT, parent, CURRENT_TIME)?;
    connection.flush()?;
    wait_for(&messages, "has focus false", KEY_WAIT)?;
    keys_reaching(connection, parent)?;
    type_key(connection, screen, KEYCODE_A)?;
    assert_eq!(
        keys_reaching(connection, parent)?,
        1,
        "the page under the pointer took the key the window holds"
    );
    view.destroy()?;
    Ok(())
}
