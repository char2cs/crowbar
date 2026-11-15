//! What a stream session woke up for.

use crate::stream_control::StreamControl;

pub(crate) enum Step {
    Frame(Option<String>),
    Control(Option<StreamControl>),
}
