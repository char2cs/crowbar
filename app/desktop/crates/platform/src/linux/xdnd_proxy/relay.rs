//! The proxy's routing: each message of a drag goes to the window, or to the XDND-aware window
//! of a web view under the pointer, and that window's answers go back to the source.

use std::sync::Arc;

use x11rb::NONE;
use x11rb::connection::Connection as _;
use x11rb::errors::ReplyError;
use x11rb::protocol::xproto::{
    AtomEnum, ClientMessageEvent, ConnectionExt as _, EventMask, PropMode,
};
use x11rb::rust_connection::RustConnection;

use crate::linux::xdnd_proxy::atoms::Atoms;
use crate::linux::xdnd_proxy::drop_target::DropTarget;

const MORE_TYPES: u32 = 1;
const VERSION_SHIFT: u32 = 24;
const MAX_TYPES: u32 = 1024;
const ACCEPTS: u32 = 1;
const SEND_EVERY_POSITION: u32 = 2;
const FORMAT: u8 = 32;

const DATA_WORDS: usize = 5;

type Data = [u32; DATA_WORDS];

pub(crate) struct Relay {
    connection: Arc<RustConnection>,
    atoms: Atoms,
    window: u32,
    proxy: u32,
    root: u32,
    source: u32,
    enter: Data,
    child: Option<u32>,
    types_copied: bool,
}

impl Relay {
    pub(crate) fn new(
        connection: Arc<RustConnection>,
        atoms: Atoms,
        windows: (u32, u32, u32),
    ) -> Self {
        let (window, proxy, root) = windows;
        Self {
            connection,
            atoms,
            window,
            proxy,
            root,
            source: NONE,
            enter: Data::default(),
            child: None,
            types_copied: false,
        }
    }

    pub(crate) fn heard(
        &mut self,
        message: &ClientMessageEvent,
    ) -> Result<(), ReplyError> {
        let (kind, data) = (message.type_, message.data.as_data32());
        let atoms = self.atoms;
        match kind {
            _ if kind == atoms.XdndEnter => self.entered(data),
            _ if kind == atoms.XdndPosition => self.moved(data),
            _ if kind == atoms.XdndLeave || kind == atoms.XdndDrop => self.ended(kind, data),
            _ if kind == atoms.XdndStatus || kind == atoms.XdndFinished => {
                self.answered(kind, data)
            },
            _ => Ok(()),
        }
    }

    fn entered(
        &mut self,
        data: Data,
    ) -> Result<(), ReplyError> {
        let [source, ..] = data;
        self.source = source;
        self.enter = data;
        self.child = None;
        self.types_copied = false;
        self.send(self.window, self.atoms.XdndEnter, data)
    }

    fn moved(
        &mut self,
        data: Data,
    ) -> Result<(), ReplyError> {
        let [_, _, point, time, action] = data;
        let ends = (self.root, self.window);
        let target = DropTarget::under(&self.connection, self.atoms.XdndAware, ends, point)?;
        let next = target.map(|target| target.window);
        if next != self.child {
            self.leave()?;
            self.enter_into(next)?;
        }
        let Some(target) = target else {
            return self.send(self.window, self.atoms.XdndPosition, data);
        };
        let position = [self.proxy, 0, target.point, time, action];
        self.send(target.window, self.atoms.XdndPosition, position)
    }

    fn ended(
        &mut self,
        kind: u32,
        data: Data,
    ) -> Result<(), ReplyError> {
        let [_, _, time, ..] = data;
        let sent = match self.child {
            Some(child) => self.send(child, kind, [self.proxy, 0, time, 0, 0]),
            None => self.send(self.window, kind, data),
        };
        if kind == self.atoms.XdndLeave {
            self.child = None;
        }
        sent
    }

    fn answered(
        &mut self,
        kind: u32,
        data: Data,
    ) -> Result<(), ReplyError> {
        let [from, flags, first, second, action] = data;
        if self.child != Some(from) {
            return Ok(());
        }
        if kind == self.atoms.XdndStatus {
            let status = [
                self.window,
                (flags & ACCEPTS) | SEND_EVERY_POSITION,
                0,
                0,
                action,
            ];
            return self.send(self.source, kind, status);
        }
        self.child = None;
        self.send(self.source, kind, [self.window, flags, first, second, 0])
    }

    fn leave(&self) -> Result<(), ReplyError> {
        let leave = self.atoms.XdndLeave;
        match self.child {
            Some(child) => self.send(child, leave, [self.proxy, 0, 0, 0, 0]),
            None => self.send(self.window, leave, [self.source, 0, 0, 0, 0]),
        }
    }

    fn enter_into(
        &mut self,
        next: Option<u32>,
    ) -> Result<(), ReplyError> {
        self.child = next;
        let Some(child) = next else {
            return self.send(self.window, self.atoms.XdndEnter, self.enter);
        };
        let [_, flags, first, second, third] = self.enter;
        if flags & MORE_TYPES != 0 && !self.types_copied {
            self.copy_types()?;
            self.types_copied = true;
        }
        let flags = (flags >> VERSION_SHIFT << VERSION_SHIFT) | (flags & MORE_TYPES);
        let enter = [self.proxy, flags, first, second, third];
        self.send(child, self.atoms.XdndEnter, enter)
    }

    fn copy_types(&self) -> Result<(), ReplyError> {
        let list = self.atoms.XdndTypeList;
        let types = self
            .connection
            .get_property(false, self.source, list, AtomEnum::ATOM, 0, MAX_TYPES)?
            .reply()?;
        self.connection.change_property(
            PropMode::REPLACE,
            self.proxy,
            list,
            AtomEnum::ATOM,
            FORMAT,
            types.value_len,
            &types.value,
        )?;
        Ok(())
    }

    fn send(
        &self,
        to: u32,
        kind: u32,
        data: Data,
    ) -> Result<(), ReplyError> {
        let message = ClientMessageEvent::new(FORMAT, to, kind, data);
        self.connection
            .send_event(false, to, EventMask::NO_EVENT, message)?;
        self.connection.flush()?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::sync::mpsc;
    use std::thread;
    use std::time::Duration;

    use x11rb::CURRENT_TIME;
    use x11rb::protocol::Event;
    use x11rb::wrapper::ConnectionExt as _;

    use crate::linux::test_support::mapped_in;
    use crate::linux::xdnd_proxy::XdndProxy;

    use super::*;

    const SIDE: u16 = 200;
    const CHILD_AT: i16 = 20;
    const CHILD_SIDE: u16 = 100;
    const INSIDE: i16 = 50;
    const VERSION: u32 = 5;
    const ACCEPTED: u32 = 1;
    const WAIT: Duration = Duration::from_secs(20);
    const WAKE: &str = "CROWBAR_TEST_WAKE";
    const HEARD: usize = 7;
    /// One type more than an `XdndEnter` carries, so the web view reads the list from its source.
    const TYPES: [&str; 4] = ["text/uri-list", "text/plain", "UTF8_STRING", "STRING"];

    #[derive(Clone, Copy)]
    struct Windows {
        window: u32,
        inner: u32,
        source: u32,
        proxy: u32,
        root: u32,
    }

    impl Windows {
        fn role(
            self,
            id: u32,
        ) -> &'static str {
            match id {
                id if id == self.window => "window",
                id if id == self.inner => "web view",
                id if id == self.source => "source",
                id if id == self.proxy => "proxy",
                _ => "another window",
            }
        }
    }

    fn atom(
        connection: &RustConnection,
        name: &str,
    ) -> u32 {
        connection
            .intern_atom(false, name.as_bytes())
            .unwrap()
            .reply()
            .unwrap()
            .atom
    }

    fn name_of(
        connection: &RustConnection,
        atom: u32,
    ) -> String {
        let reply = connection.get_atom_name(atom).unwrap().reply().unwrap();
        String::from_utf8_lossy(&reply.name).into_owned()
    }

    fn send(
        connection: &RustConnection,
        to: u32,
        (kind, data): (&str, Data),
    ) {
        let message = ClientMessageEvent::new(FORMAT, to, atom(connection, kind), data);
        connection
            .send_event(false, to, EventMask::NO_EVENT, message)
            .unwrap();
        connection.flush().unwrap();
    }

    fn proxy_of(
        connection: &RustConnection,
        window: u32,
    ) -> u32 {
        let named = atom(connection, "XdndProxy");
        let reply = connection
            .get_property(false, window, named, AtomEnum::WINDOW, 0, 1)
            .unwrap()
            .reply()
            .unwrap();
        reply
            .value32()
            .and_then(|mut ids| ids.next())
            .unwrap_or(NONE)
    }

    fn windows(connection: &RustConnection) -> Windows {
        let root = connection.setup().roots[0].root;
        let window = mapped_in(connection, root, (0, 0, SIDE, SIDE));
        let child = mapped_in(
            connection,
            window,
            (CHILD_AT, CHILD_AT, CHILD_SIDE, CHILD_SIDE),
        );
        let inner = mapped_in(connection, child, (0, 0, CHILD_SIDE, CHILD_SIDE));
        let aware = atom(connection, "XdndAware");
        connection
            .change_property32(PropMode::REPLACE, child, aware, AtomEnum::ATOM, &[VERSION])
            .unwrap();
        let source = mapped_in(connection, root, (0, 0, 1, 1));
        let types: Vec<u32> = TYPES.iter().map(|name| atom(connection, name)).collect();
        let list = atom(connection, "XdndTypeList");
        connection
            .change_property32(PropMode::REPLACE, source, list, AtomEnum::ATOM, &types)
            .unwrap();
        connection.get_input_focus().unwrap().reply().unwrap();
        Windows {
            window,
            inner,
            source,
            proxy: NONE,
            root,
        }
    }

    fn start_drag(
        connection: &RustConnection,
        windows: Windows,
    ) {
        let types: Vec<u32> = TYPES.iter().map(|name| atom(connection, name)).collect();
        let flags = (VERSION << VERSION_SHIFT) | MORE_TYPES;
        let enter = [windows.source, flags, types[0], types[1], types[2]];
        send(connection, windows.proxy, ("XdndEnter", enter));
        let at = connection
            .translate_coordinates(windows.window, windows.root, INSIDE, INSIDE)
            .unwrap()
            .reply()
            .unwrap();
        let point = DropTarget::at(NONE, &at).point;
        let copy = atom(connection, "XdndActionCopy");
        let position = [windows.source, 0, point, CURRENT_TIME, copy];
        send(connection, windows.proxy, ("XdndPosition", position));
    }

    fn answer(
        connection: &RustConnection,
        windows: Windows,
        heard: &str,
    ) {
        let copy = atom(connection, "XdndActionCopy");
        let reply = match heard {
            "XdndPosition" => ("XdndStatus", [windows.inner, ACCEPTED, 0, 0, copy]),
            "XdndStatus" => ("XdndDrop", [windows.source, 0, CURRENT_TIME, 0, 0]),
            "XdndDrop" => ("XdndFinished", [windows.inner, ACCEPTED, copy, 0, 0]),
            _ => return,
        };
        send(connection, windows.proxy, reply);
    }

    fn wake_after_the_deadline(
        window: u32,
        done: mpsc::Receiver<()>,
    ) {
        thread::spawn(move || wake_unless_done(window, &done));
    }

    fn wake_unless_done(
        window: u32,
        done: &mpsc::Receiver<()>,
    ) {
        if done.recv_timeout(WAIT).is_ok() {
            return;
        }
        let (connection, _) = RustConnection::connect(None).unwrap();
        send(&connection, window, (WAKE, [0; 5]));
    }

    fn client_message(connection: &RustConnection) -> ClientMessageEvent {
        match connection.wait_for_event().unwrap() {
            Event::ClientMessage(event) => event,
            _ => client_message(connection),
        }
    }

    fn converse(
        connection: &RustConnection,
        windows: Windows,
    ) -> Vec<(&'static str, String, &'static str)> {
        let mut heard = Vec::new();
        while heard.len() < HEARD {
            let event = client_message(connection);
            let kind = name_of(connection, event.type_);
            assert_ne!(kind, WAKE, "the drag stalled after {heard:?}");
            let [from, ..] = event.data.as_data32();
            answer(connection, windows, &kind);
            heard.push((windows.role(event.window), kind, windows.role(from)));
        }
        heard
    }

    #[test]
    #[ignore = "needs display"]
    fn a_drag_over_a_web_view_leaves_the_window_reaches_the_web_view_and_its_answers_the_source() {
        let (connection, _) = RustConnection::connect(None).unwrap();
        let mut windows = windows(&connection);
        let _proxy = XdndProxy::install(windows.window).unwrap();
        windows.proxy = proxy_of(&connection, windows.window);
        let (done, deadline) = mpsc::channel();
        wake_after_the_deadline(windows.window, deadline);
        start_drag(&connection, windows);
        let heard = converse(&connection, windows);
        done.send(()).unwrap_or_default();
        let types: Vec<String> = TYPES.map(str::to_owned).to_vec();
        let list = atom(&connection, "XdndTypeList");
        let copied = connection
            .get_property(false, windows.proxy, list, AtomEnum::ATOM, 0, MAX_TYPES)
            .unwrap()
            .reply()
            .unwrap();
        let copied: Vec<String> = copied
            .value32()
            .into_iter()
            .flatten()
            .map(|atom| name_of(&connection, atom))
            .collect();
        let at = |at, kind: &str, from| (at, kind.to_owned(), from);
        assert_eq!(
            heard,
            [
                at("window", "XdndEnter", "source"),
                at("window", "XdndLeave", "source"),
                at("web view", "XdndEnter", "proxy"),
                at("web view", "XdndPosition", "proxy"),
                at("source", "XdndStatus", "window"),
                at("web view", "XdndDrop", "proxy"),
                at("source", "XdndFinished", "window"),
            ]
        );
        assert_eq!(copied, types, "the web view reads the types from the proxy");
    }
}
