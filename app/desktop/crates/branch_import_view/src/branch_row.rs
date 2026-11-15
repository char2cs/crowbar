//! One branch of the list: protected and imported ones are shown faded, the rest can be picked.

use gpui::{ClickEvent, Div, ElementId, ParentElement, SharedString, Styled, div};
use shell_data::{Availability, ImportBranch};
use ui::{Button, Checkbox, Icon, IconName, Palette, Pose};

use crate::row_choice::{RowChoice, Toggle};
use crate::row_look::{GAP, ICON, ROW_HEIGHT, TEXT, UNAVAILABLE_OPACITY};

fn name(branch: &ImportBranch) -> Div {
    div()
        .flex_1()
        .min_w_0()
        .truncate()
        .text_size(TEXT)
        .child(SharedString::from(branch.name.clone()))
}

fn faded(
    icon: Icon,
    branch: &ImportBranch,
) -> Div {
    div()
        .flex()
        .items_center()
        .gap(GAP)
        .h(ROW_HEIGHT)
        .opacity(UNAVAILABLE_OPACITY)
        .child(icon.size(ICON))
        .child(name(branch))
}

fn lock_button(
    index: usize,
    locked: Pose,
    toggle: Toggle,
) -> Button {
    let icon = match locked {
        Pose::On => IconName::Lock,
        Pose::Off => IconName::LockOpen,
    };
    Button::icon_only(ElementId::from(("import-lock", index)), icon)
        .on_click(move |_: &ClickEvent, window, cx| toggle(window, cx))
}

fn pickable(
    index: usize,
    branch: &ImportBranch,
    choice: RowChoice,
) -> Div {
    let RowChoice {
        picked,
        locked,
        toggle,
        toggle_lock,
    } = choice;
    let row = div()
        .flex()
        .items_center()
        .gap(GAP)
        .h(ROW_HEIGHT)
        .child(
            Checkbox::new(ElementId::from(("import-pick", index)), picked)
                .on_change(move |_, window, cx| toggle(window, cx)),
        )
        .child(name(branch));
    if picked == Pose::Off {
        return row;
    }
    row.child(lock_button(index, locked, toggle_lock))
}

pub(crate) fn branch_row(
    index: usize,
    branch: &ImportBranch,
    (choice, palette): (RowChoice, &Palette),
) -> Div {
    match branch.availability {
        Availability::Protected => faded(Icon::new(IconName::Lock), branch),
        Availability::Imported => {
            faded(Icon::new(IconName::Check).color(palette.success()), branch)
        },
        Availability::Importable => pickable(index, branch, choice),
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Context, IntoElement, Render, TestAppContext, Window};

    use super::*;

    struct Rows;

    fn choice() -> RowChoice {
        RowChoice {
            picked: Pose::On,
            locked: Pose::Off,
            toggle: Box::new(|_, _| {}),
            toggle_lock: Box::new(|_, _| {}),
        }
    }

    fn branch(
        name: &str,
        availability: Availability,
    ) -> ImportBranch {
        ImportBranch {
            name: name.to_owned(),
            availability,
        }
    }

    impl Render for Rows {
        fn render(
            &mut self,
            _: &mut Window,
            cx: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let palette = *ui::Theme::of(cx).palette(ui::Surface::Window);
            div()
                .child(branch_row(
                    0,
                    &branch("a", Availability::Importable),
                    (choice(), &palette),
                ))
                .child(branch_row(
                    1,
                    &branch("b", Availability::Imported),
                    (choice(), &palette),
                ))
                .child(branch_row(
                    2,
                    &branch("c", Availability::Protected),
                    (choice(), &palette),
                ))
        }
    }

    #[gpui::test]
    fn every_availability_draws_a_row(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let (_, cx) = cx.add_window_view(|_, _| Rows);
        cx.run_until_parked();
    }
}
