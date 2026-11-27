use std::io;

pub(crate) trait Sink: Send {
    fn append(
        &mut self,
        text: &str,
    ) -> io::Result<()>;
}
