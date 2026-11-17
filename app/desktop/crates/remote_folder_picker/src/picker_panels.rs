use gpui::{
    AnyElement, Context, Div, InteractiveElement, IntoElement, ParentElement,
    StatefulInteractiveElement, Styled, div, uniform_list,
};
use shell_data::{BrowseEntry, FolderListing, ROOT_PATH};
use ui::{Button, ButtonSize, ButtonVariant, Palette, Pose};

use crate::folder_row::FolderRow;
use crate::folder_rows::{children_of, rows};
use crate::picker_widgets::{centred, highlighted, inert, toggler};
use crate::remote_folder_picker::RemoteFolderPicker;
use crate::row_look::{BANNER_TINT, EMPTY_ROOT, GAP, LOADING_ROOT, ROW_HEIGHT, TEXT, TREE_HEIGHT};
use crate::shown::Shown;
use crate::tree_row::tree_row;

impl RemoteFolderPicker {
    pub(crate) fn row_element(
        &self,
        (index, row): (usize, &FolderRow),
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let host = self.source.host_name();
        let FolderRow::Entry { entry, .. } = row else {
            return tree_row(index, row, (&host, palette), inert()).into_any_element();
        };
        let toggle = toggler(&cx.entity(), &entry.path);
        let picked = entry.clone();
        let chosen = self
            .selected
            .as_ref()
            .is_some_and(|held| held.path == entry.path);
        let styled = tree_row(index, row, (&host, palette), toggle);
        highlighted(styled, palette, Pose::from(chosen))
            .id(("folder-row", index))
            .on_click(cx.listener(move |this, _, _, cx| this.choose(&picked, cx)))
            .into_any_element()
    }

    pub(crate) fn tree(
        &self,
        entries: &[BrowseEntry],
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let shown = Shown {
            source: self.source.as_ref(),
            expanded: &self.expanded,
        };
        let all = rows(ROOT_PATH, entries, &shown, 1);
        let count = all.len();
        let palette = *palette;
        uniform_list(
            "remote-folders",
            count,
            cx.processor(move |this, range: std::ops::Range<usize>, _, cx| {
                range
                    .filter_map(|index| all.get(index).map(|row| (index, row)))
                    .map(|pair| this.row_element(pair, &palette, cx))
                    .collect::<Vec<_>>()
            }),
        )
        .h(TREE_HEIGHT)
        .into_any_element()
    }

    pub(crate) fn failure(
        &self,
        (reason, palette): (&str, &Palette),
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let host = self.source.host_name();
        let mut retry = Button::new("folders-retry", "Retry")
            .variant(ButtonVariant::Outline)
            .size(ButtonSize::Sm)
            .on_click(cx.listener(|this, _, _, cx| this.retry_root(cx)));
        if !self.source.can_ask() {
            retry = retry.disabled();
        }
        centred(
            palette,
            format!("Couldn't list folders on {host}: {reason}"),
        )
        .flex_col()
        .gap(GAP)
        .child(retry)
        .into_any_element()
    }

    pub(crate) fn retry_root(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.source.retry(ROOT_PATH);
        cx.notify();
    }

    pub(crate) fn root(
        &self,
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let host = self.source.host_name();
        match self.source.listing(ROOT_PATH) {
            None if !self.source.can_ask() => {
                centred(palette, format!("Folders load once {host} answers.")).into_any_element()
            },
            None | Some(FolderListing::Loading) => {
                centred(palette, LOADING_ROOT).into_any_element()
            },
            Some(FolderListing::Failed(reason)) => self.failure((&reason, palette), cx),
            Some(FolderListing::Loaded(entries)) if children_of(ROOT_PATH, &entries).is_empty() => {
                centred(palette, EMPTY_ROOT).into_any_element()
            },
            Some(FolderListing::Loaded(entries)) => self.tree(&entries, palette, cx),
        }
    }

    pub(crate) fn footer(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let label = self.selected.as_ref().map_or_else(
            || "Select".to_owned(),
            |entry| format!("Select \"{}\"", entry.name),
        );
        let mut select = Button::new("folders-select", label)
            .size(ButtonSize::Sm)
            .on_click(cx.listener(|this, _, _, cx| this.confirm(cx)));
        if self.selected.is_none() || !self.source.can_ask() {
            select = select.disabled();
        }
        let cancel = Button::new("folders-cancel", "Cancel")
            .variant(ButtonVariant::Ghost)
            .size(ButtonSize::Sm)
            .on_click(cx.listener(|_, _, _, cx| Self::cancel(cx)));
        div().flex().gap(GAP).child(cancel).child(select)
    }

    pub(crate) fn banner(
        &self,
        palette: &Palette,
    ) -> Option<Div> {
        if self.source.can_ask() {
            return None;
        }
        let host = self.source.host_name();
        let text = format!("{host} is not answering. Its folders load again once it does.");
        Some(
            div()
                .px(GAP * 2.0)
                .h(ROW_HEIGHT)
                .flex()
                .items_center()
                .bg(palette.warning().opacity(BANNER_TINT))
                .text_size(TEXT)
                .child(text),
        )
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use shell_data::FolderListing;

    use super::*;
    use crate::test_support::{Events, open, reachable_fake};

    #[gpui::test]
    fn the_root_failure_offers_a_retry_that_asks_again(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let (fake, events) = (reachable_fake(), Events::default());
        fake.listings
            .borrow_mut()
            .insert("/".to_owned(), FolderListing::Failed("denied".into()));
        let (picker, cx) = open(cx, &fake, &events);
        cx.run_until_parked();
        picker.update(cx, RemoteFolderPicker::retry_root);
        assert_eq!(*fake.retried.borrow(), ["/".to_owned()]);
    }
}
