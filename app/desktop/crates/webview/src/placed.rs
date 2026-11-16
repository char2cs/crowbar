use wry::Rect;

use crate::visibility::Visibility;

use crate::error::Result;

pub(crate) trait Placed {
    fn bounds(&self) -> Result<Rect>;

    fn set_visibility(
        &self,
        visibility: Visibility,
    ) -> Result<()>;

    fn set_bounds(
        &self,
        bounds: Rect,
    ) -> Result<()>;
}

impl Placed for wry::WebView {
    fn bounds(&self) -> Result<Rect> {
        Ok(wry::WebView::bounds(self)?)
    }

    fn set_visibility(
        &self,
        visibility: Visibility,
    ) -> Result<()> {
        Ok(wry::WebView::set_visible(
            self,
            visibility == Visibility::Shown,
        )?)
    }

    fn set_bounds(
        &self,
        bounds: Rect,
    ) -> Result<()> {
        Ok(wry::WebView::set_bounds(self, bounds)?)
    }
}

pub(crate) fn show_where_it_was(view: &impl Placed) -> Result<()> {
    let placed = view.bounds()?;
    view.set_visibility(Visibility::Shown)?;
    view.set_bounds(placed)
}

#[cfg(test)]
mod tests {
    use std::cell::{Cell, RefCell};

    use wry::dpi::{LogicalPosition, LogicalSize};

    use super::*;

    struct ForgetfulView {
        at: Cell<Rect>,
        calls: RefCell<Vec<&'static str>>,
    }

    fn rect(x: f64) -> Rect {
        Rect {
            position: LogicalPosition::new(x, 0.0).into(),
            size: LogicalSize::new(10.0, 10.0).into(),
        }
    }

    impl Placed for ForgetfulView {
        fn bounds(&self) -> Result<Rect> {
            Ok(self.at.get())
        }

        fn set_visibility(
            &self,
            _visibility: Visibility,
        ) -> Result<()> {
            self.calls.borrow_mut().push("show");
            self.at.set(rect(0.0));
            Ok(())
        }

        fn set_bounds(
            &self,
            bounds: Rect,
        ) -> Result<()> {
            self.calls.borrow_mut().push("place");
            self.at.set(bounds);
            Ok(())
        }
    }

    #[test]
    fn a_view_whose_window_forgets_its_place_when_shown_is_put_back() {
        let view = ForgetfulView {
            at: Cell::new(rect(25.0)),
            calls: RefCell::new(Vec::new()),
        };
        show_where_it_was(&view).unwrap();
        assert_eq!(*view.calls.borrow(), ["show", "place"], "order");
        assert_eq!(format!("{:?}", view.at.get()), format!("{:?}", rect(25.0)));
    }
}
