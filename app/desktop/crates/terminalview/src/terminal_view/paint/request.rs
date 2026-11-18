//! What the canvas hands to a paint pass: its bounds and the frame prepaint produced.

use gpui::{Bounds, Pixels};

use crate::terminal_view::paint::frame::Frame;

pub(crate) struct Request {
    pub(crate) bounds: Bounds<Pixels>,
    pub(crate) frame: Option<Frame>,
}

impl Request {
    pub(crate) fn new(
        bounds: Bounds<Pixels>,
        frame: Option<Frame>,
    ) -> Self {
        Self { bounds, frame }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_request_without_a_frame_carries_only_its_bounds() {
        let request = Request::new(Bounds::default(), None);
        assert!(request.frame.is_none());
        assert_eq!(request.bounds, Bounds::default());
    }
}
