//! The active space's name: set tight, and renamed by a double click.

use gpui::{
    AnyElement, ClickEvent, Context, Div, Entity, FontWeight, InteractiveElement, IntoElement,
    ParentElement, StatefulInteractiveElement, Styled, Window, div, px,
};
use sidebar::{Editing, Project, ProjectId};
use ui::{Palette, Surface, TextInput, Theme, TrackedText, tracking};

use crate::sidebar::Sidebar;

const NAME_SIZE: f32 = 19.0;
const NAME_LINE: f32 = 24.0;

impl Sidebar {
    pub(super) fn space_label(
        &self,
        current: &Project,
        theme: Theme,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let face = (theme.heading_family(), tracking::TIGHT);
        let palette = theme.palette(Surface::Chrome);
        let editing = Editing::RenamingProject(current.id.clone());
        let field = self
            .inline
            .as_ref()
            .filter(|_| self.tree.editing() == &editing);
        if let Some(edit) = field {
            return name_field(edit.input.clone()).into_any_element();
        }
        let space = (current.id.clone(), current.name.clone());
        spaced_name_cell(current, face, palette)
            .id("space-name")
            .debug_selector(|| "space-name".into())
            .on_click(cx.listener(move |this, event: &ClickEvent, window, cx| {
                this.name_clicked(&space, event, (window, cx));
            }))
            .into_any_element()
    }

    fn name_clicked(
        &mut self,
        space: &(ProjectId, String),
        click: &ClickEvent,
        (window, cx): (&mut Window, &mut Context<'_, Self>),
    ) {
        if click.click_count() != 2 {
            return;
        }
        self.start_project_rename((space.0.clone(), &space.1), window, cx);
    }
}

fn spaced_name_cell(
    current: &Project,
    (family, em): (&'static str, f32),
    palette: &Palette,
) -> Div {
    div()
        .flex_1()
        .min_w_0()
        .truncate()
        .font_family(family)
        .text_size(px(NAME_SIZE))
        .line_height(px(NAME_LINE))
        .font_weight(FontWeight::NORMAL)
        .text_color(palette.foreground())
        .child(TrackedText::new(current.name.clone(), em))
}

fn name_field(input: Entity<TextInput>) -> Div {
    div().flex_1().min_w_0().child(input)
}

#[cfg(test)]
mod tests {
    use gpui::{
        EntityInputHandler as _, InteractiveElement as _, ParentElement as _, Styled as _, div,
    };
    use sidebar::{Command, HostId, Mark, Project, ProjectId};
    use ui::{Surface, Theme};

    use super::{NAME_SIZE, spaced_name_cell};
    use crate::test_support::mount;
    use crate::test_support::prelude::*;

    const SEED: &str = "Crowbar Seed";
    /// Tailwind `tracking-tight`, in em.
    const TIGHT: f32 = -0.025;

    fn name_width(
        cx: &mut TestAppContext,
        em: f32,
    ) -> f32 {
        text_width(cx, SEED, em)
    }

    fn text_width(
        cx: &mut TestAppContext,
        name: &str,
        em: f32,
    ) -> f32 {
        let seed = Project {
            id: ProjectId::from("seed"),
            name: name.to_owned(),
            mark: Mark::Fallback,
            host: HostId::from("local"),
        };
        let window = mount(cx, move |_, cx| {
            let theme = Theme::of(cx);
            let cell = spaced_name_cell(
                &seed,
                (theme.heading_family(), em),
                theme.palette(Surface::Chrome),
            );
            div().flex().child(
                div()
                    .flex_none()
                    .debug_selector(|| "switcher-name".into())
                    .child(cell),
            )
        });
        let bounds = window.debug_bounds("switcher-name").unwrap_or_default();
        f32::from(bounds.size.width)
    }

    #[gpui::test]
    fn the_space_name_is_set_tight_as_develop_does(cx: &mut TestAppContext) {
        let tight = name_width(cx, TIGHT);
        let loose = name_width(cx, 0.0);
        let glyphs = f32::from(u16::try_from(SEED.chars().count()).expect("a short name"));
        let narrower = loose - tight;
        let expected = glyphs * -TIGHT * NAME_SIZE;
        assert!(
            (narrower - expected).abs() < 1.0,
            "{narrower} vs {expected}"
        );
    }

    fn double_click_the_name(cx: &mut gpui::VisualTestContext) {
        let name = cx.debug_bounds("space-name").unwrap_or_default();
        let at = name.origin + point(px(4.0), name.size.height / 2.0);
        cx.simulate_event(gpui::MouseDownEvent {
            button: gpui::MouseButton::Left,
            position: at,
            click_count: 2,
            ..Default::default()
        });
        cx.simulate_event(gpui::MouseUpEvent {
            button: gpui::MouseButton::Left,
            position: at,
            click_count: 2,
            ..Default::default()
        });
        cx.run_until_parked();
    }

    #[gpui::test]
    fn double_clicking_the_space_name_renames_the_space(cx: &mut TestAppContext) {
        let (recorder, _, cx) = open(cx, DemoShell::rich());
        double_click_the_name(cx);
        cx.simulate_input("Atlas");
        cx.simulate_keystrokes("enter");
        let renamed = Command::RenameProject {
            project: ProjectId::from("inv"),
            name: "Atlas".into(),
        };
        assert_eq!(recorder.sent(), vec![renamed]);
    }

    #[gpui::test]
    fn the_rename_field_sets_the_name_exactly_as_the_label_does(cx: &mut TestAppContext) {
        let name = "Inventory service";
        let expected = text_width(cx, name, TIGHT);
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        double_click_the_name(cx);
        let input = sidebar.read_with(cx, |this, _| {
            this.inline.as_ref().expect("a field").input.clone()
        });
        input.update_in(cx, |input, window, cx| {
            input.replace_text_in_range(Some(0..64), name, window, cx);
        });
        cx.run_until_parked();
        let length = name.len();
        let bounds = input.update_in(cx, |input, window, cx| {
            input.bounds_for_range(0..length, gpui::Bounds::default(), window, cx)
        });
        let width = f32::from(bounds.expect("a painted line").size.width);
        assert!((width - expected).abs() < 1.0, "{width} vs {expected}");
    }

    #[gpui::test]
    fn confirming_the_same_space_name_sends_nothing(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::rich());
        double_click_the_name(cx);
        cx.simulate_keystrokes("enter");
        assert!(sidebar.read_with(cx, |this, _| this.inline.is_none()));
        assert_eq!(recorder.sent(), vec![]);
    }
}
