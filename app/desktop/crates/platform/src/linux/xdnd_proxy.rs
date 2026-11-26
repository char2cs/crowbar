//! Drags from other applications onto the web views a window embeds.
//!
//! A drag source talks only to the top-level window under the pointer, so a web view, a child
//! window, never hears a drag. The window names a hidden window of ours as its `XdndProxy`
//! (XDND spec, "Proxy"); that window hands each message on to the window itself, or to the web
//! view under the pointer, naming itself as the source, and passes the web view's answers back.

mod atoms;
mod drop_target;
mod relay;

use std::sync::Arc;
use std::thread;

use x11rb::connection::Connection as _;
use x11rb::errors::{ConnectionError, ReplyError, ReplyOrIdError};
use x11rb::protocol::Event;
use x11rb::protocol::xproto::{
    AtomEnum, ClientMessageEvent, ConnectionExt as _, CreateWindowAux, EventMask, PropMode,
    WindowClass,
};
use x11rb::rust_connection::RustConnection;
use x11rb::wrapper::ConnectionExt as _;
use x11rb::{COPY_DEPTH_FROM_PARENT, COPY_FROM_PARENT};

use crate::error::Error;
use crate::linux::xdnd_proxy::atoms::Atoms;
use crate::linux::xdnd_proxy::relay::Relay;

const THREAD: &str = "crowbar-xdnd-proxy";
const OFF_SCREEN: i16 = -1;
const SIDE: u16 = 1;

/// The proxy of one window's drags, served on a thread and a connection of its own until it is
/// dropped.
#[derive(Debug)]
pub struct XdndProxy {
    connection: Arc<RustConnection>,
    atoms: Atoms,
    window: u32,
    proxy: u32,
}

impl XdndProxy {
    /// Serves the drags that reach `window`, an X window id.
    ///
    /// # Errors
    /// [`Error::X11Connect`] when no X server answers, [`Error::Proxy`] when the proxy window
    /// cannot be made or named, and [`Error::Thread`] when its thread cannot start.
    pub fn install(window: u32) -> Result<Self, Error> {
        let (connection, screen) = RustConnection::connect(None)?;
        let root = connection
            .setup()
            .roots
            .get(screen)
            .map(|screen| screen.root);
        let root = root.ok_or(Error::Proxy(ConnectionError::UnknownError.into()))?;
        let connection = Arc::new(connection);
        let (atoms, proxy) = name_proxy(&connection, root, window).map_err(Error::Proxy)?;
        let relay = Relay::new(Arc::clone(&connection), atoms, (window, proxy, root));
        let served = Arc::clone(&connection);
        thread::Builder::new()
            .name(THREAD.into())
            .spawn(move || serve(&served, relay, proxy))
            .map_err(Error::Thread)?;
        Ok(Self {
            connection,
            atoms,
            window,
            proxy,
        })
    }
}

impl Drop for XdndProxy {
    fn drop(&mut self) {
        let connection = &self.connection;
        // PLATFORM: linux X11 flush only sends; the round trip makes every client see the removal.
        let removed = connection
            .delete_property(self.window, self.atoms.XdndProxy)
            .and_then(|_| connection.destroy_window(self.proxy))
            .map_err(ReplyError::from)
            .and_then(|_| connection.sync());
        if let Err(error) = removed {
            tracing::debug!(%error, "the drag proxy outlived its connection");
        }
    }
}

fn name_proxy(
    connection: &RustConnection,
    root: u32,
    window: u32,
) -> Result<(Atoms, u32), ReplyOrIdError> {
    let atoms = Atoms::new(connection)?.reply()?;
    let proxy = connection.generate_id()?;
    let attributes = CreateWindowAux::new()
        .override_redirect(1)
        .event_mask(EventMask::STRUCTURE_NOTIFY);
    connection
        .create_window(
            COPY_DEPTH_FROM_PARENT,
            proxy,
            root,
            OFF_SCREEN,
            OFF_SCREEN,
            SIDE,
            SIDE,
            0,
            WindowClass::INPUT_OUTPUT,
            COPY_FROM_PARENT,
            &attributes,
        )?
        .check()?;
    for named in [proxy, window] {
        connection
            .change_property32(
                PropMode::REPLACE,
                named,
                atoms.XdndProxy,
                AtomEnum::WINDOW,
                &[proxy],
            )?
            .check()?;
    }
    Ok((atoms, proxy))
}

fn serve(
    connection: &RustConnection,
    mut relay: Relay,
    proxy: u32,
) {
    let events = std::iter::from_fn(|| connection.wait_for_event().ok());
    let messages = events
        .take_while(|event| !matches!(event, Event::DestroyNotify(gone) if gone.window == proxy))
        .filter_map(|event| client_message(&event));
    messages
        .filter_map(|message| relay.heard(&message).err())
        .for_each(|error| tracing::warn!(%error, "a drag message could not be passed on"));
}

fn client_message(event: &Event) -> Option<ClientMessageEvent> {
    let Event::ClientMessage(message) = event else {
        return None;
    };
    Some(*message)
}

#[cfg(test)]
mod tests {
    use x11rb::NONE;

    use super::*;

    fn proxy_named_on(
        connection: &RustConnection,
        window: u32,
    ) -> u32 {
        let atoms = Atoms::new(connection).unwrap().reply().unwrap();
        let reply = connection
            .get_property(false, window, atoms.XdndProxy, AtomEnum::WINDOW, 0, 1)
            .unwrap()
            .reply()
            .unwrap();
        reply
            .value32()
            .and_then(|mut ids| ids.next())
            .unwrap_or(NONE)
    }

    #[test]
    #[ignore = "needs display"]
    fn the_proxy_names_itself_on_the_window_and_itself_until_it_is_dropped() {
        let (connection, screen) = RustConnection::connect(None).unwrap();
        let root = connection.setup().roots[screen].root;
        let window = connection.generate_id().unwrap();
        let aux = CreateWindowAux::new();
        connection
            .create_window(
                0,
                window,
                root,
                0,
                0,
                SIDE,
                SIDE,
                0,
                WindowClass::INPUT_ONLY,
                0,
                &aux,
            )
            .unwrap()
            .check()
            .unwrap();
        let installed = XdndProxy::install(window).unwrap();
        let proxy = proxy_named_on(&connection, window);
        assert_ne!(proxy, NONE, "the window names a proxy");
        assert_eq!(
            proxy_named_on(&connection, proxy),
            proxy,
            "the proxy names itself"
        );
        drop(installed);
        assert_eq!(
            proxy_named_on(&connection, window),
            NONE,
            "dropped, the name goes"
        );
    }
}
