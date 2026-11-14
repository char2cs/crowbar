//! The delete confirmation dialog.

use gpui::{AnyElement, Context, Div, FontWeight, IntoElement, ParentElement, Styled, div, px};
use ui::font::MONO_FAMILY;
use ui::{Button, ButtonVariant, Dialog, DialogSize, Palette, Surface, Theme};

use crate::command::Command;
use crate::confirmation::Confirmation;
use crate::removal_target::RemovalTarget;
use crate::sidebar::Sidebar;
use crate::work_at_risk::WorkAtRisk;

fn risk_row(work: &WorkAtRisk, palette: &Palette) -> Div {
    let branch = div()
        .font_family(MONO_FAMILY)
        .text_color(palette.foreground())
        .child(work.branch.clone());
    let summary = div()
        .text_color(palette.muted_foreground())
        .child(work.summary());
    div().flex().flex_col().child(branch).child(summary)
}

fn body(confirmation: &Confirmation, palette: &Palette) -> Div {
    let (strong, rest) = confirmation.lead();
    let lead = div()
        .child(
            div()
                .text_color(palette.foreground())
                .font_weight(FontWeight::BOLD)
                .child(strong),
        )
        .child(rest);
    div()
        .flex()
        .flex_col()
        .gap(px(12.0))
        .text_size(px(14.0))
        .text_color(palette.muted_foreground())
        .child(lead)
        .children(
            confirmation
                .at_risk
                .iter()
                .map(|work| risk_row(work, palette)),
        )
}

impl Sidebar {
    pub(crate) fn confirm_layer(&self, cx: &mut Context<'_, Self>) -> Option<AnyElement> {
        let confirmation = self.tree.confirmation()?;
        let palette = Theme::of(cx).palette(Surface::Window);
        let cancel = Button::new("confirm-cancel", "Cancel")
            .variant(ButtonVariant::Ghost)
            .on_click(cx.listener(|this, _, _, cx| this.cancel_confirmation(cx)));
        let delete = Button::new("confirm-delete", confirmation.confirm_label())
            .variant(ButtonVariant::Destructive)
            .on_click(cx.listener(|this, _, _, cx| this.accept_confirmation(cx)));
        let sidebar = cx.entity();
        Some(
            Dialog::new("sidebar-confirm")
                .title(confirmation.title())
                .size(DialogSize::Md)
                .body(body(confirmation, palette))
                .footer(div().flex().gap(px(8.0)).child(cancel).child(delete))
                .on_close(move |_, cx| sidebar.update(cx, Self::cancel_confirmation))
                .into_any_element(),
        )
    }

    fn cancel_confirmation(&mut self, cx: &mut Context<'_, Self>) {
        self.tree.cancel_confirmation();
        cx.notify();
    }

    fn accept_confirmation(&mut self, cx: &mut Context<'_, Self>) {
        let Some(Confirmation { target, .. }) = self.tree.accept_confirmation() else {
            return;
        };
        let command = match target {
            RemovalTarget::Row(row) => Command::CommitRemoval(row),
            RemovalTarget::Project(project) => Command::DeleteProject(project),
        };
        self.send(command, cx);
        cx.notify();
    }
}

#[cfg(test)]
#[path = "confirm_view_tests.rs"]
mod tests;
