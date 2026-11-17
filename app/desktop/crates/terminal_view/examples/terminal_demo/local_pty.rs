//! A local PTY wired to a `TerminalPeer`, standing in for the daemon's websocket.

use std::collections::HashMap;
use std::thread;

use alacritty_terminal::event::OnResize;
use alacritty_terminal::event::WindowSize;
use alacritty_terminal::tty::{self, Options, Pty};
use rustix::fd::{AsFd, OwnedFd};
use rustix::fs::{OFlags, fcntl_getfl, fcntl_setfl};
use rustix::io::{dup, read, write};
use terminal::{ClientMessage, DaemonFrame};
use terminal_view::TerminalPeer;

const READ_CHUNK: usize = 65_536;
const CELL_PIXELS: u16 = 8;
const LINE_PIXELS: u16 = 17;

pub(crate) fn spawn_local_pty(
    peer: TerminalPeer,
    width: u16,
    height: u16,
) -> std::io::Result<()> {
    let env = HashMap::from([
        ("TERM".to_owned(), "xterm-256color".to_owned()),
        ("COLORTERM".to_owned(), "truecolor".to_owned()),
    ]);
    let options = Options {
        env,
        ..Options::default()
    };
    let columns = width / CELL_PIXELS;
    let rows = height / LINE_PIXELS;
    let pty = tty::new(&options, window_size(columns, rows), 0)?;
    let reader = blocking_clone(pty.file())?;
    let writer = blocking_clone(pty.file())?;
    let reading = peer.clone();
    thread::spawn(move || pump_output(&reader, &reading));
    thread::spawn(move || pump_input(pty, &writer, &peer));
    Ok(())
}

fn blocking_clone(file: &impl AsFd) -> std::io::Result<OwnedFd> {
    let clone = dup(file)?;
    let flags = fcntl_getfl(&clone)?;
    fcntl_setfl(&clone, flags - OFlags::NONBLOCK)?;
    Ok(clone)
}

fn pump_output(
    reader: &OwnedFd,
    peer: &TerminalPeer,
) {
    let mut buffer = vec![0_u8; READ_CHUNK];
    while forward_chunk(reader, &mut buffer, peer) {}
}

fn forward_chunk(
    reader: &OwnedFd,
    buffer: &mut [u8],
    peer: &TerminalPeer,
) -> bool {
    let count = read(reader, &mut *buffer).unwrap_or(0);
    if count == 0 {
        peer.deliver_blocking(DaemonFrame::Exit(0));
        return false;
    }
    let chunk = buffer.get(..count).unwrap_or_default().to_vec();
    peer.deliver_blocking(DaemonFrame::Output(chunk))
}

fn pump_input(
    mut pty: Pty,
    writer: &OwnedFd,
    peer: &TerminalPeer,
) {
    let mut messages = std::iter::from_fn(|| peer.next_message_blocking());
    messages.all(|message| handle_message(message, &mut pty, writer));
}

fn handle_message(
    message: ClientMessage,
    pty: &mut Pty,
    writer: &OwnedFd,
) -> bool {
    match message {
        ClientMessage::Input(text) => write_all(writer, text.as_bytes()),
        ClientMessage::Resize { columns, rows } => {
            pty.on_resize(window_size(columns, rows));
            true
        },
        ClientMessage::Theme { .. } => true,
    }
}

fn write_all(
    writer: &OwnedFd,
    mut bytes: &[u8],
) -> bool {
    while !bytes.is_empty() {
        let Ok(written) = write(writer, bytes) else {
            return false;
        };
        bytes = bytes.get(written..).unwrap_or_default();
    }
    true
}

fn window_size(
    columns: u16,
    rows: u16,
) -> WindowSize {
    WindowSize {
        num_lines: rows,
        num_cols: columns,
        cell_width: CELL_PIXELS,
        cell_height: LINE_PIXELS,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn window_size_carries_the_grid_and_cell_size() {
        let size = window_size(80, 24);
        assert_eq!((size.num_cols, size.num_lines), (80, 24));
        assert_eq!(size.cell_width, CELL_PIXELS);
    }
}
