use std::io;

pub(crate) trait LogSink: Send {
    fn append(
        &mut self,
        text: &str,
    ) -> io::Result<()>;
}
