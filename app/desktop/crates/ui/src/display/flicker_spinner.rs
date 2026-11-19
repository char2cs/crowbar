//! The flip-dot loading indicator of rows and tabs.

mod dot;
mod markup;
mod playback;
mod sprite;
mod sprites;

use gpui::{
    App, Div, ElementId, Hsla, IntoElement, ParentElement, Pixels, RenderOnce, Styled, Window, div,
    px,
};

use crate::display::flicker_spinner::dot::Dot;
use crate::display::flicker_spinner::playback::Playback;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::spacing;

const OPACITY: f32 = 0.8;

/// A grid of dots stepping through one of the bundled flip-dot sprites, picked at random per
/// spinner, one frame every 90 ms.
///
/// It repaints only when its frame changes and keeps playing under reduced motion.
#[derive(IntoElement)]
pub struct FlickerSpinner {
    id: ElementId,
    size: Pixels,
    surface: Surface,
    color: Option<Hsla>,
}

impl FlickerSpinner {
    /// A 16px spinner; `id` keeps its sprite and timeline across frames.
    pub fn new(id: impl Into<ElementId>) -> Self {
        Self {
            id: id.into(),
            size: spacing::STEP_4,
            surface: Surface::default(),
            color: None,
        }
    }

    /// Sets the edge length.
    #[must_use]
    pub fn size(
        mut self,
        size: Pixels,
    ) -> Self {
        self.size = size;
        self
    }

    /// Takes the muted text colour of `surface`'s palette.
    #[must_use]
    pub fn on(
        mut self,
        surface: Surface,
    ) -> Self {
        self.surface = surface;
        self
    }

    /// Draws the dots in `color` instead of the muted text colour.
    #[must_use]
    pub fn color(
        mut self,
        color: Hsla,
    ) -> Self {
        self.color = Some(color);
        self
    }
}

fn dot_view(
    dot: &Dot,
    frame: usize,
    (scale, color): (f32, Hsla),
) -> Div {
    div()
        .absolute()
        .left(px(dot.left() * scale))
        .top(px(dot.top() * scale))
        .size(px(dot.diameter() * scale))
        .rounded_full()
        .bg(color.opacity(dot.opacity(frame)))
}

impl RenderOnce for FlickerSpinner {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let muted = Theme::of(cx)
            .palette(self.surface)
            .muted_foreground()
            .opacity(OPACITY);
        let color = self.color.unwrap_or(muted);
        let playback = window.use_keyed_state(self.id, cx, |_, cx| Playback::start(cx));
        let shown = playback.update(cx, Playback::frame);
        let dots = shown.into_iter().flat_map(|(sprite, frame)| {
            let scale = f32::from(self.size) / sprite.width();
            sprite
                .dots()
                .iter()
                .map(|dot| dot_view(dot, frame, (scale, color)))
                .collect::<Vec<_>>()
        });
        div().relative().flex_none().size(self.size).children(dots)
    }
}

opaque_debug!(FlickerSpinner);

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;
    use std::time::Duration;

    use gpui::{Context, Render, TestAppContext};

    use super::*;
    use crate::test_support::probe::measure;

    const STEP: Duration = Duration::from_millis(90);
    const JUST_BEFORE: Duration = Duration::from_millis(1);

    struct Host {
        draws: Rc<Cell<usize>>,
    }

    impl Render for Host {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            self.draws.set(self.draws.get() + 1);
            FlickerSpinner::new("spinner")
        }
    }

    #[gpui::test]
    fn the_spinner_requests_frames_only_at_its_steps(cx: &mut TestAppContext) {
        cx.update(|cx| cx.set_global(Theme::default()));
        let draws = Rc::new(Cell::new(0));
        let counter = Rc::clone(&draws);
        let (_, cx) = cx.add_window_view(|_, _| Host { draws: counter });
        let settled = draws.get();
        cx.executor()
            .advance_clock(STEP.saturating_sub(JUST_BEFORE));
        assert_eq!(draws.get(), settled, "no frame between steps");
        cx.executor().advance_clock(JUST_BEFORE);
        assert_eq!(draws.get(), settled + 1, "one frame at the step");
        cx.executor().advance_clock(STEP * 10);
        assert_eq!(draws.get(), settled + 11, "one frame per step");
    }

    #[gpui::test]
    fn the_default_size_is_sixteen_pixels(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| FlickerSpinner::new("s"));
        assert_eq!(f32::from(bounds.size.width), 16.0);
        let small = measure(cx, |_, _| FlickerSpinner::new("s").size(px(14.0)));
        assert_eq!(f32::from(small.size.height), 14.0);
    }
}
