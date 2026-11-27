use std::cell::Cell;

use wry::{Rect, WebView};

use crate::backend::native::linux::gtk_state::placed::Placed;
use crate::error::Result;
use crate::view::Visibility;

pub(crate) struct RememberedPlace {
    // PLATFORM: GTK reports a position that lags a queued move, so the last place asked for is kept here.
    wanted: Cell<Rect>,
}

impl RememberedPlace {
    pub(crate) fn new(wanted: Rect) -> Self {
        Self {
            wanted: Cell::new(wanted),
        }
    }

    pub(crate) fn place(
        &self,
        view: &impl Placed,
        bounds: Rect,
    ) -> Result<()> {
        view.set_bounds(bounds)?;
        self.wanted.set(bounds);
        Ok(())
    }

    pub(crate) fn size(
        &self,
        scale: f64,
    ) -> (f64, f64) {
        let size = self.wanted.get().size.to_logical::<f64>(scale);
        (size.width, size.height)
    }

    pub(crate) fn show(
        &self,
        view: &impl Placed,
    ) -> Result<()> {
        view.set_visibility(Visibility::Shown)?;
        view.set_bounds(self.wanted.get())
    }
}

impl Placed for WebView {
    fn set_visibility(
        &self,
        visibility: Visibility,
    ) -> Result<()> {
        Ok(WebView::set_visible(self, visibility == Visibility::Shown)?)
    }

    fn set_bounds(
        &self,
        bounds: Rect,
    ) -> Result<()> {
        Ok(WebView::set_bounds(self, bounds)?)
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;

    use wry::dpi::{LogicalPosition, LogicalSize};

    use super::*;

    fn rect(x: f64) -> Rect {
        Rect {
            position: LogicalPosition::new(x, 0.0).into(),
            size: LogicalSize::new(10.0, 10.0).into(),
        }
    }

    struct LaggingView {
        settled: Cell<Rect>,
        queued: RefCell<Vec<Rect>>,
    }

    impl LaggingView {
        fn settle(&self) -> String {
            let latest = self.queued.borrow_mut().drain(..).next_back();
            self.settled.set(latest.unwrap_or(self.settled.get()));
            format!("{:?}", self.settled.get())
        }
    }

    impl Placed for LaggingView {
        fn set_visibility(
            &self,
            _visibility: Visibility,
        ) -> Result<()> {
            self.queued.borrow_mut().clear();
            self.settled.set(rect(0.0));
            Ok(())
        }

        fn set_bounds(
            &self,
            bounds: Rect,
        ) -> Result<()> {
            self.queued.borrow_mut().push(bounds);
            Ok(())
        }
    }

    fn view() -> LaggingView {
        LaggingView {
            settled: Cell::new(rect(25.0)),
            queued: RefCell::new(Vec::new()),
        }
    }

    #[test]
    fn a_view_moved_and_shown_in_one_go_lands_where_it_was_last_moved() {
        let (view, place) = (view(), RememberedPlace::new(rect(25.0)));
        place.place(&view, rect(40.0)).unwrap();
        place.show(&view).unwrap();
        assert_eq!(view.settle(), format!("{:?}", rect(40.0)), "moved place");
    }

    #[test]
    fn a_view_shown_without_a_move_lands_where_it_was_created() {
        let (view, place) = (view(), RememberedPlace::new(rect(25.0)));
        place.show(&view).unwrap();
        assert_eq!(view.settle(), format!("{:?}", rect(25.0)), "created place");
    }
}
