//! A local PTY wired to a `TerminalPeer`, standing in for the daemon's websocket.

use std::collections::HashMap;
use std::fs::File;
use std::io::{Read, Write};
use std::thread;

use alacritty_terminal::event::OnResize;
use alacritty_terminal::event::WindowSize;
use alacritty_terminal::tty::{self, Options, Pty};
use rustix::fs::{OFlags, fcntl_getfl, fcntl_setfl};
use terminal::{ClientMessage, DaemonFrame};
use terminal_view::TerminalPeer;

const READ_CHUNK: usize = 65_536;
const CELL_PIXELS: f32 = 8.0;
const LINE_PIXELS: f32 = 17.0;

pub(crate) fn spawn_local_pty(
    peer: TerminalPeer,
    width: f32,
    height: f32,
) -> std::io::Result<()> {
    let env = HashMap::from([
        ("TERM".to_owned(), "xterm-256color".to_owned()),
        ("COLORTERM".to_owned(), "truecolor".to_owned()),
    ]);
    let options = Options {
        env,
        ..Options::default()
    };
    let columns = (width / CELL_PIXELS) as u16;
    let rows = (height / LINE_PIXELS) as u16;
    let pty = tty::new(&options, window_size(columns, rows), 0)?;
    let reader = blocking_clone(pty.file())?;
    let writer = blocking_clone(pty.file())?;
    let reading = peer.clone();
    thread::spawn(move || pump_output(reader, &reading));
    thread::spawn(move || pump_input(pty, writer, &peer));
    Ok(())
}

fn blocking_clone(file: &File) -> std::io::Result<File> {
    let clone = file.try_clone()?;
    let flags = fcntl_getfl(&clone)?;
    fcntl_setfl(&clone, flags - OFlags::NONBLOCK)?;
    Ok(clone)
}

fn pump_output(
    mut reader: File,
    peer: &TerminalPeer,
) {
    let mut buffer = vec![0_u8; READ_CHUNK];
    while forward_chunk(&mut reader, &mut buffer, peer) {}
}

fn forward_chunk(
    reader: &mut File,
    buffer: &mut [u8],
    peer: &TerminalPeer,
) -> bool {
    let read = reader.read(buffer).unwrap_or(0);
    if read == 0 {
        peer.deliver_blocking(DaemonFrame::Exit(0));
        return false;
    }
    let chunk = buffer.get(..read).unwrap_or_default().to_vec();
    peer.deliver_blocking(DaemonFrame::Output(chunk))
}

fn pump_input(
    mut pty: Pty,
    mut writer: File,
    peer: &TerminalPeer,
) {
    let mut messages = std::iter::from_fn(|| peer.next_message_blocking());
    messages.all(|message| handle_message(message, &mut pty, &mut writer));
}

fn handle_message(
    message: ClientMessage,
    pty: &mut Pty,
    writer: &mut File,
) -> bool {
    match message {
        ClientMessage::Input(text) => writer.write_all(text.as_bytes()).is_ok(),
        ClientMessage::Resize { columns, rows } => {
            pty.on_resize(window_size(columns, rows));
            true
        },
        ClientMessage::Theme { .. } => true,
    }
}

fn window_size(
    columns: u16,
    rows: u16,
) -> WindowSize {
    WindowSize {
        num_lines: rows,
        num_cols: columns,
        cell_width: CELL_PIXELS as u16,
        cell_height: LINE_PIXELS as u16,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn window_size_carries_the_grid_and_cell_size() {
        let size = window_size(80, 24);
        assert_eq!((size.num_cols, size.num_lines), (80, 24));
        assert_eq!(size.cell_width, CELL_PIXELS as u16);
    }
}
