use crate::error::Error;
use crate::shell::command::Command;

pub trait Sink {
    fn dispatch(
        &self,
        command: Command,
    ) -> Result<(), Error>;
}
