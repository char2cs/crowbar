use gpui::{Context, IntoElement, Render, Window, div};

use crate::control::divider::axis::Axis;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct SashDrag {
    pub(crate) axis: Axis,
}

impl SashDrag {
    pub fn axis(&self) -> Axis {
        self.axis
    }
}

impl Render for SashDrag {
    fn render(
        &mut self,
        _window: &mut Window,
        _cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        div()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reports_its_axis() {
        let drag = SashDrag {
            axis: Axis::Horizontal,
        };
        assert_eq!(drag.axis(), Axis::Horizontal);
    }
}
