use std::fmt::Debug;
use std::time::{Instant, SystemTime};

pub(crate) trait Clock: Debug + Send + Sync {
    fn wall(&self) -> SystemTime;
    fn instant(&self) -> Instant;
}
