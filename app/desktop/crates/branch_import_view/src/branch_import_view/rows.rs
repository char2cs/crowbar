//! The branch list of the dialog and its rows.

use gpui::{
    Context, Div, Entity, IntoElement, ParentElement, SharedString, Styled, div, uniform_list,
};
use shell_data::ImportBranch;
use ui::{Button, ButtonSize, Palette, Pose, Theme};

use super::BranchImportView;
use crate::branch_filter::{empty_message, matching};
use crate::branch_row::branch_row;
use crate::import_hint;
use crate::row_choice::{RowChoice, Toggle};
use crate::row_look::{GAP, LEAD, LIST_HEIGHT, TEXT};

fn import_button(
    picked: usize,
    cx: &mut Context<'_, BranchImportView>,
) -> Button {
    let button = Button::new("import-start", "Import")
        .size(ButtonSize::Sm)
        .on_click(cx.listener(|this, _, _, cx| this.start(cx)));
    if picked == 0 {
        return button.disabled();
    }
    button
}

fn pick_handler(
    view: &Entity<BranchImportView>,
    name: &str,
) -> Toggle {
    let (view, name) = (view.clone(), name.to_owned());
    Box::new(move |_, cx| view.update(cx, |this, cx| this.pick(&name, cx)))
}

fn lock_handler(
    view: &Entity<BranchImportView>,
    name: &str,
) -> Toggle {
    let (view, name) = (view.clone(), name.to_owned());
    Box::new(move |_, cx| view.update(cx, |this, cx| this.lock(&name, cx)))
}

fn muted(palette: &Palette) -> Div {
    div().text_size(TEXT).text_color(palette.muted_foreground())
}

impl BranchImportView {
    pub(crate) fn row_at(
        &self,
        branch: &ImportBranch,
        (index, palette): (usize, &Palette),
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let view = cx.entity();
        let choice = RowChoice {
            picked: Pose::from(self.selection.is_picked(&branch.name)),
            locked: Pose::from(self.selection.is_locked(&branch.name)),
            toggle: pick_handler(&view, &branch.name),
            toggle_lock: lock_handler(&view, &branch.name),
        };
        branch_row(index, branch, (choice, palette)).font_family(Theme::of(cx).mono_family())
    }

    pub(crate) fn row_if(
        &self,
        rows: &[ImportBranch],
        (index, palette): (usize, &Palette),
        cx: &mut Context<'_, Self>,
    ) -> Option<Div> {
        let row = rows.get(index)?;
        Some(self.row_at(row, (index, palette), cx))
    }

    pub(crate) fn list(
        rows: Vec<ImportBranch>,
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let count = rows.len();
        let palette = *palette;
        uniform_list(
            "import-branches",
            count,
            cx.processor(move |this, range: std::ops::Range<usize>, _, cx| {
                range
                    .filter_map(|index| this.row_if(&rows, (index, &palette), cx))
                    .collect::<Vec<_>>()
            }),
        )
        .h(LIST_HEIGHT)
    }

    pub(crate) fn body(
        &self,
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let listing = self.source.listing();
        let rows = matching(&listing, &self.filter);
        let message = empty_message(&listing, rows.len());
        let plan = self.source.plan(&self.selection.picked());
        let shown = match message {
            Some(text) => muted(palette).h(LIST_HEIGHT).child(text).into_any_element(),
            None => Self::list(rows, palette, cx).into_any_element(),
        };
        div()
            .flex()
            .flex_col()
            .gap(GAP)
            .child(muted(palette).child(LEAD))
            .child(self.input.clone())
            .child(shown)
            .child(muted(palette).child(SharedString::from(import_hint::of(plan))))
            .child(import_button(self.selection.picked().len(), cx))
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use shell_data::Availability;

    use shell_data::BranchListing;
    use ui::Surface;

    use super::*;
    use crate::branch_import_view::tests::{Imported, branch, open};

    #[gpui::test]
    fn a_row_exists_only_for_an_index_inside_the_list(cx: &mut TestAppContext) {
        let rows = vec![branch("a", Availability::Importable)];
        let listing = BranchListing::Ready(rows.clone());
        let (view, cx) = open(cx, listing, &Imported::default());
        let palette = cx.update(|_, cx| *Theme::of(cx).palette(Surface::Window));
        let found = view.update(cx, |view, cx| {
            let inside = view.row_if(&rows, (0, &palette), cx).is_some();
            let outside = view.row_if(&rows, (1, &palette), cx).is_none();
            (inside, outside)
        });
        assert_eq!(found, (true, true));
    }
}
