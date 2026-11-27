use gpui::Pixels;

use super::{Button, ButtonSize, ButtonVariant, IconName, Surface};

impl Button {
    #[must_use]
    pub fn icon(
        mut self,
        icon: IconName,
    ) -> Self {
        self.icon = Some(icon);
        self
    }

    #[must_use]
    pub fn icon_size(
        mut self,
        size: Pixels,
    ) -> Self {
        self.icon_size = Some(size);
        self
    }

    #[must_use]
    pub fn variant(
        mut self,
        variant: ButtonVariant,
    ) -> Self {
        self.variant = variant;
        self
    }

    #[must_use]
    pub fn size(
        mut self,
        size: ButtonSize,
    ) -> Self {
        self.size = size;
        self
    }

    #[must_use]
    pub fn on(
        mut self,
        surface: Surface,
    ) -> Self {
        self.surface = surface;
        self
    }
}

#[cfg(test)]
mod tests {
    use std::cell::{Cell, RefCell};
    use std::rc::Rc;
    use std::time::{Duration, Instant};

    use super::*;
    use crate::foundation::Motion;
    use crate::foundation::Pose;
    use crate::foundation::motion::hover;
    use crate::foundation::motion::{CubicBezier, ToggleState, Tween};
    use crate::foundation::tokens::timing;
    use crate::test_support::probe::open;

    #[test]
    fn the_builders_choose_icon_variant_size_and_surface() {
        let button = Button::new("b", "Save")
            .icon(IconName::Plus)
            .icon_size(gpui::px(14.0))
            .variant(ButtonVariant::Ghost)
            .size(ButtonSize::Sm)
            .on(Surface::Window);
        assert_eq!(button.icon, Some(IconName::Plus));
        assert_eq!(button.icon_size, Some(gpui::px(14.0)));
        assert_eq!(button.variant, ButtonVariant::Ghost);
        assert_eq!(button.size, ButtonSize::Sm);
        assert_eq!(button.surface, Surface::Window);
    }

    type Captured = Rc<RefCell<Option<gpui::Entity<ToggleState>>>>;

    fn tracked(
        id: &gpui::ElementId,
        window: &mut gpui::Window,
        cx: &mut gpui::App,
    ) -> gpui::Entity<ToggleState> {
        let component = gpui::ElementId::Name(std::any::type_name::<Button>().into());
        window.with_global_id(component, |_, window| {
            hover::track(id, Pose::Off, window, cx)
        })
    }

    fn stepper(
        disabled: Rc<Cell<Pose>>,
        captured: Captured,
    ) -> impl Fn(&mut gpui::Window, &mut gpui::App) -> Button {
        move |window, cx| {
            let id = gpui::ElementId::from("step");
            *captured.borrow_mut() = Some(tracked(&id, window, cx));
            let button = Button::icon_only(id, IconName::Plus);
            gpui::prelude::FluentBuilder::when(button, disabled.get() == Pose::On, Button::disabled)
        }
    }

    #[gpui::test]
    fn a_pointer_that_leaves_while_disabled_leaves_no_hover_behind(cx: &mut gpui::TestAppContext) {
        let (disabled, captured): (Rc<Cell<Pose>>, Captured) = Default::default();
        let cx = open(cx, stepper(Rc::clone(&disabled), Rc::clone(&captured)));
        let at = |x: f32| gpui::point(gpui::px(x), gpui::px(x));
        cx.simulate_mouse_move(at(10.0), None, gpui::Modifiers::none());
        cx.run_until_parked();
        assert_eq!(settled(&captured, cx), Some(1.0), "hovered");
        disabled.set(Pose::On);
        cx.update(|window, _| window.refresh());
        cx.run_until_parked();
        cx.simulate_mouse_move(at(90.0), None, gpui::Modifiers::none());
        cx.run_until_parked();
        assert_eq!(settled(&captured, cx), Some(0.0), "left");
    }

    fn settled(
        captured: &Captured,
        cx: &mut gpui::VisualTestContext,
    ) -> Option<f32> {
        let state = captured.borrow().clone()?;
        let later = Instant::now() + Duration::from_secs(1);
        let tween = Tween::new(timing::DEFAULT, CubicBezier::DEFAULT);
        Some(state.read_with(cx, |s, _| s.position(tween, Motion::default(), later).value))
    }
}
