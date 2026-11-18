//! The merge box of the Changes panel: commit form, strategy, merge and the reasons it may not.

use gpui::prelude::FluentBuilder as _;
use gpui::{
    AnyElement, Context, Div, FontWeight, InteractiveElement, IntoElement, ParentElement,
    StatefulInteractiveElement, Styled, div, px,
};
use sidebar::{AfterMerge, Command, MergeBox, MergeFacts, MergeStrategy};
use ui::{
    Button, ButtonSize, ButtonVariant, Checkbox, Icon, IconName, Palette, Select, Surface, Theme,
    radius,
};

use crate::panels::changes::merge_draft::after_merge;
use crate::panels::changes::merge_lines::{note, path_row};
use crate::sidebar::Sidebar;
use crate::sidebar::{ICON_LARGE, SPACE_LG, SPACE_MD, SPACE_SM, SPACE_XS, TEXT_BODY, TEXT_SMALL};

const LIST_MAX_HEIGHT: f32 = 160.0;
const LINE_HEIGHT: f32 = 16.0;
const DELETE_LABEL: &str = "Delete this branch and its chats after merging";

impl Sidebar {
    pub(crate) fn merge_card(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let merge = self.data.merge_box()?;
        let facts = MergeFacts::of(&merge);
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let header = div()
            .flex()
            .items_center()
            .gap(SPACE_MD)
            .p(SPACE_LG)
            .text_size(TEXT_BODY)
            .font_weight(FontWeight::MEDIUM)
            .child(
                Icon::new(IconName::GitMerge)
                    .size(ICON_LARGE)
                    .color(palette.muted_foreground()),
            )
            .child(facts.title());
        let mut card = div()
            .flex()
            .flex_col()
            .mx(SPACE_SM)
            .my(SPACE_XS)
            .rounded(radius::XL)
            .border_1()
            .border_color(palette.border())
            .bg(palette.card())
            .text_color(palette.foreground())
            .child(header);
        if facts.shows_content() {
            card = card.child(self.merge_content((&merge, facts), palette, cx));
        }
        Some(card.into_any_element())
    }

    fn merge_content(
        &mut self,
        (merge, facts): (&MergeBox, MergeFacts<'_>),
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let mut content = div()
            .flex()
            .flex_col()
            .gap(SPACE_MD)
            .px(SPACE_LG)
            .pb(SPACE_LG);
        if facts.offers_commit() {
            content = content.child(self.commit_form((merge, facts), cx));
        }
        content = content
            .child(Self::strategy_select(merge.strategy, cx))
            .child(Self::merge_button(facts, cx))
            .child(self.delete_option(cx));
        let notes = [
            merge
                .blocked
                .clone()
                .map(|text| (text, palette.muted_foreground())),
            merge
                .working
                .clone()
                .map(|text| (text, palette.muted_foreground())),
            merge
                .error
                .clone()
                .map(|text| (text, palette.destructive())),
        ];
        for (text, color) in notes.into_iter().flatten() {
            content = content.child(note(text, color));
        }
        if facts.offers_stop() {
            content = content.child(Self::stop_button(cx));
        }
        content
    }

    fn commit_form(
        &mut self,
        (merge, facts): (&MergeBox, MergeFacts<'_>),
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let uncommitted = &merge.uncommitted;
        let paths = self.merge.paths(uncommitted);
        let sidebar = cx.entity();
        let rows = uncommitted
            .iter()
            .map(|path| path_row(path, &paths, &sidebar));
        let list = div()
            .id("merge-paths")
            .flex()
            .flex_col()
            .gap(SPACE_XS)
            .max_h(px(LIST_MAX_HEIGHT))
            .overflow_y_scroll()
            .children(rows);
        let commit = Button::new("merge-commit", facts.commit_label(paths.len()))
            .size(ButtonSize::Sm)
            .variant(ButtonVariant::Secondary)
            .on_click(cx.listener(|this, _, _, cx| this.commit_changes(cx)));
        let drafted = !self.merge.message.read(cx).text().trim().is_empty();
        let ready = drafted && facts.can_commit(paths.len());
        let commit = commit.when(!ready, Button::disabled);
        div()
            .flex()
            .flex_col()
            .gap(SPACE_MD)
            .child(list)
            .child(self.merge.message.clone())
            .child(commit)
    }

    fn merge_now(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let after = self.merge.after;
        self.send(Command::Merge { after }, cx);
    }

    pub(crate) fn toggle_path(
        &mut self,
        path: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.merge.toggle(path);
        cx.notify();
    }

    fn choose_strategy(
        &mut self,
        index: usize,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(strategy) = MergeStrategy::ALL.get(index) else {
            return;
        };
        self.send(Command::SetMergeStrategy(*strategy), cx);
    }

    fn set_after(
        &mut self,
        after: AfterMerge,
        cx: &mut Context<'_, Self>,
    ) {
        self.merge.after = after;
        cx.notify();
    }

    pub(crate) fn commit_changes(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(merge) = self.data.merge_box() else {
            return;
        };
        let message = self.merge.message.read(cx).text().trim().to_owned();
        let paths = self.merge.paths(&merge.uncommitted);
        if message.is_empty() || !MergeFacts::of(&merge).can_commit(paths.len()) {
            return;
        }
        self.send(Command::CommitChanges { message, paths }, cx);
        self.merge
            .message
            .update(cx, |field, cx| field.set_text("", cx));
        self.merge.skipped.clear();
    }

    fn strategy_select(
        current: MergeStrategy,
        cx: &mut Context<'_, Self>,
    ) -> Select {
        let labels = MergeStrategy::ALL
            .iter()
            .map(|s| s.label().into())
            .collect();
        let at = MergeStrategy::ALL.iter().position(|s| *s == current);
        let sidebar = cx.entity();
        Select::new("merge-strategy", labels, at)
            .on(Surface::Chrome)
            .compact()
            .fill()
            .on_change(move |index, _, cx| {
                sidebar.update(cx, |this, cx| this.choose_strategy(index, cx));
            })
    }

    fn merge_button(
        facts: MergeFacts<'_>,
        cx: &mut Context<'_, Self>,
    ) -> Button {
        let button = Button::new("merge-confirm", facts.confirm_label())
            .size(ButtonSize::Sm)
            .on_click(cx.listener(|this, _, _, cx| this.merge_now(cx)));
        if !facts.can_merge() {
            return button.disabled();
        }
        button
    }

    fn delete_option(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let checked = self.merge.after == AfterMerge::DeleteBranch;
        let sidebar = cx.entity();
        let checkbox = Checkbox::new("merge-delete", checked)
            .on(Surface::Chrome)
            .on_change(move |value, _, cx| {
                sidebar.update(cx, |this, cx| this.set_after(after_merge(value), cx));
            });
        div()
            .flex()
            .items_start()
            .gap(SPACE_MD)
            .text_size(TEXT_SMALL)
            .line_height(px(LINE_HEIGHT))
            .child(checkbox)
            .child(div().flex_1().min_w_0().child(DELETE_LABEL))
    }

    fn stop_button(cx: &mut Context<'_, Self>) -> Button {
        Button::new("merge-stop", "Stop and merge")
            .size(ButtonSize::Sm)
            .variant(ButtonVariant::Outline)
            .on_click(cx.listener(|this, _, _, cx| this.send(Command::StopAndMerge, cx)))
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{AfterMerge, Command, MergeBox, MergeWork};

    use crate::sidebar::Sidebar;
    use crate::test_support::prelude::*;

    fn open_box() -> MergeBox {
        MergeBox {
            parent: Some("main".into()),
            uncommitted: vec!["a.rs".into(), "b.rs".into()],
            ..MergeBox::default()
        }
    }

    #[gpui::test]
    fn the_buttons_send_the_commands_the_box_describes(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoData::reference().with_merge_box(open_box()));
        sidebar.update(cx, |this, cx| {
            this.toggle_path("b.rs", cx);
            this.set_after(AfterMerge::DeleteBranch, cx);
            this.merge
                .message
                .update(cx, |field, cx| field.set_text("fix", cx));
            this.commit_changes(cx);
            this.merge_now(cx);
        });
        let want = vec![
            Command::CommitChanges {
                message: "fix".into(),
                paths: vec!["a.rs".into()],
            },
            Command::Merge {
                after: AfterMerge::DeleteBranch,
            },
        ];
        assert_eq!(recorder.sent(), want);
    }

    #[gpui::test]
    fn submitting_the_message_commits_the_ticked_files(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoData::reference().with_merge_box(open_box()));
        sidebar.update(cx, |this, cx| {
            this.merge
                .message
                .update(cx, |field, cx| field.set_text("fix", cx));
            this.merge
                .message
                .update(cx, |_, cx| cx.emit(ui::TextInputEvent::Submitted));
        });
        let want = vec![Command::CommitChanges {
            message: "fix".into(),
            paths: vec!["a.rs".into(), "b.rs".into()],
        }];
        assert_eq!(recorder.sent(), want);
    }

    #[gpui::test]
    fn a_running_commit_blocks_another(cx: &mut TestAppContext) {
        let busy = MergeBox {
            work: MergeWork::Committing,
            ..open_box()
        };
        let (recorder, sidebar, cx) = open(cx, DemoData::reference().with_merge_box(busy));
        sidebar.update(cx, |this, cx| {
            this.merge
                .message
                .update(cx, |field, cx| field.set_text("again", cx));
            this.commit_changes(cx);
        });
        assert_eq!(recorder.sent(), Vec::<Command>::new());
    }

    #[gpui::test]
    fn an_empty_message_commits_nothing(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoData::reference().with_merge_box(open_box()));
        sidebar.update(cx, Sidebar::commit_changes);
        assert_eq!(recorder.sent(), Vec::<Command>::new());
    }
}
