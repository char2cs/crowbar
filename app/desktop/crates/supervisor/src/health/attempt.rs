use crate::error::Error;

pub(crate) enum Attempt {
    Served(Option<u32>),
    Refused(Error),
    Waiting,
    Failed,
}
