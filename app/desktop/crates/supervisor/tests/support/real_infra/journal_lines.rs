use std::io;

use tokio::sync::mpsc::{UnboundedReceiver, UnboundedSender, unbounded_channel};
use tracing::subscriber::DefaultGuard;

pub(crate) fn listen() -> (UnboundedReceiver<String>, DefaultGuard) {
    let (sender, lines) = unbounded_channel();
    let subscriber = tracing_subscriber::fmt()
        .with_writer(move || JournalLines(sender.clone()))
        .finish();
    (lines, tracing::subscriber::set_default(subscriber))
}

pub(crate) struct JournalLines(UnboundedSender<String>);

impl io::Write for JournalLines {
    fn write(
        &mut self,
        bytes: &[u8],
    ) -> io::Result<usize> {
        let line = String::from_utf8_lossy(bytes).into_owned();
        self.0.send(line).map_err(io::Error::other)?;
        Ok(bytes.len())
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}
