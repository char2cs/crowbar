//! The merge box of the Changes panel: commit form, strategy, merge and the reasons it may not.

use gpui::prelude::FluentBuilder as _;
use gpui::{
    AnyElement, Context, Div, Entity, InteractiveElement, IntoElement, ParentElement, SharedString,
    StatefulInteractiveElement, Styled, div, px,
};
use ui::{
    Button, ButtonSize, ButtonVariant, Checkbox, Icon, IconName, Palette, Select, Surface, Theme,
    radius,
};

use crate::after_merge::AfterMerge;
use crate::command::Command;
use crate::merge_box::MergeBox;
use crate::merge_draft::commit_label;
use crate::merge_facts::MergeFacts;
use crate::merge_strategy::MergeStrategy;
use crate::scale::{ICON_LARGE, SPACE_LG, SPACE_MD, SPACE_SM, SPACE_XS, TEXT_LARGE, TEXT_SMALL};
use crate::sidebar::Sidebar;

const LIST_MAX_HEIGHT: f32 = 160.0;
const SELECT_WIDTH: f32 = 250.0;
const DELETE_LABEL: &str = "Delete this branch and its chats after merging";

fn path_row(
    path: &str,
    ticked: &[String],
    sidebar: &Entity<Sidebar>,
) -> Div {
    let (target, owner) = (path.to_owned(), sidebar.clone());
    let id = SharedString::from(format!("merge-path-{path}"));
    let checkbox = Checkbox::new(id, ticked.iter().any(|candidate| candidate == path))
        .on(Surface::Chrome)
        .on_change(move |_, _, cx| {
            owner.update(cx, |this, cx| this.toggle_path(&target, cx));
        });
    div()
        .flex()
        .items_center()
        .gap(SPACE_MD)
        .text_size(TEXT_SMALL)
        .child(checkbox)
        .child(div().truncate().child(path.to_owned()))
}

fn note(
    text: String,
    color: gpui::Hsla,
) -> Div {
    div().text_size(TEXT_SMALL).text_color(color).child(text)
}

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
            .text_size(TEXT_LARGE)
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
            .bg(palette.control_surface())
            .text_color(palette.control_foreground())
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
            content = content.child(self.commit_form(&merge.uncommitted, cx));
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
        uncommitted: &[String],
        cx: &mut Context<'_, Self>,
    ) -> Div {
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
        let commit = Button::new("merge-commit", commit_label(paths.len()))
            .size(ButtonSize::Sm)
            .variant(ButtonVariant::Secondary)
            .on_click(cx.listener(|this, _, _, cx| this.commit_changes(cx)));
        let commit = commit.when(paths.is_empty(), Button::disabled);
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

    fn toggle_path(
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

    fn commit_changes(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(merge) = self.data.merge_box() else {
            return;
        };
        let message = self.merge.message.read(cx).text().trim().to_owned();
        let paths = self.merge.paths(&merge.uncommitted);
        if message.is_empty() || paths.is_empty() {
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
            .width(px(SELECT_WIDTH))
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
                sidebar.update(cx, |this, cx| this.set_after(AfterMerge::from(value), cx));
            });
        div()
            .flex()
            .items_center()
            .gap(SPACE_MD)
            .text_size(TEXT_SMALL)
            .child(checkbox)
            .child(DELETE_LABEL)
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
    use crate::after_merge::AfterMerge;
    use crate::command::Command;
    use crate::merge_box::MergeBox;
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
    fn an_empty_message_commits_nothing(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoData::reference().with_merge_box(open_box()));
        sidebar.update(cx, Sidebar::commit_changes);
        assert_eq!(recorder.sent(), Vec::<Command>::new());
    }
}
