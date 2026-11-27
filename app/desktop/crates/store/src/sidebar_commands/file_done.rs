use crate::sidebar_commands::file_error::FileError;

pub type FileDone<T> = Box<dyn FnOnce(Result<T, FileError>) + Send>;
