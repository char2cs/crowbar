//! A repository header's click-to-edit mark: develop's `EditableRepoIcon`.

use std::rc::Rc;

use gpui::{
    AnyElement, App, Context, ElementId, Entity, InteractiveElement as _, IntoElement, MouseButton,
    ParentElement, Pixels, SharedString, Styled, WeakEntity, Window, div,
};
use sidebar::{AvatarTint, Command, IconTarget, Mark, Row, RowId, SpaceIcon};
use ui::{Placement, Popover};

use crate::column::switcher::IconEditor;
use crate::column::switcher::edit_pencil::{GROUP, edit_pencil};
use crate::sidebar::Sidebar;

fn shown_icon(mark: Option<&Mark>) -> SpaceIcon {
    match mark {
        Some(Mark::Emoji(text)) => SpaceIcon::Emoji(text.clone()),
        Some(Mark::Image(path)) => SpaceIcon::Image(path.to_string_lossy().into_owned()),
        _ => SpaceIcon::Default,
    }
}

fn letter_of(row: &Row) -> Mark {
    if let Some(Mark::Letter { letter, tint }) = row.mark {
        return Mark::Letter { letter, tint };
    }
    let letter = row
        .label
        .chars()
        .next()
        .map_or('R', |c| c.to_ascii_uppercase());
    Mark::Letter {
        letter,
        tint: AvatarTint::default(),
    }
}

fn send(
    sidebar: &WeakEntity<Sidebar>,
    command: Command,
    cx: &mut App,
) {
    sidebar
        .update(cx, |this, cx| this.send(command, cx))
        .unwrap_or_else(|error| tracing::debug!(%error, "sidebar closed before the repo icon"));
}

fn repo_editor(
    sidebar: WeakEntity<Sidebar>,
    row: RowId,
    cx: &mut Context<'_, IconEditor>,
) -> IconEditor {
    let (picker, fetcher, reader) = (sidebar.clone(), row.clone(), sidebar.clone());
    let target = IconTarget::Repo(row.clone());
    let busy = Rc::new(move |cx: &App| {
        reader
            .upgrade()
            .is_some_and(|this| this.read(cx).data.icon_busy(&target))
    });
    let pick = Rc::new(move |icon: SpaceIcon, cx: &mut App| {
        let row = row.clone();
        send(&picker, Command::SetRepoIcon { row, icon }, cx);
    });
    let fetch = Rc::new(move |cx: &mut App| {
        send(&sidebar, Command::UseGithubAvatar(fetcher.clone()), cx);
    });
    IconEditor::new((pick, busy), cx).with_github(fetch)
}

fn keyed_editor(
    (window, cx): (&mut Window, &mut App),
    (sidebar, row): (&WeakEntity<Sidebar>, &RowId),
) -> Entity<IconEditor> {
    let key = SharedString::from(format!("repo-icon-editor-{}", row.as_str()));
    let (sidebar, row) = (sidebar.clone(), row.clone());
    window.use_keyed_state(ElementId::from(key), cx, move |_, cx| {
        repo_editor(sidebar, row, cx)
    })
}

pub(crate) fn editable_mark(
    row: &Row,
    (mark, size): (AnyElement, Pixels),
    sidebar: WeakEntity<Sidebar>,
) -> AnyElement {
    let key = SharedString::from(format!("repo-icon-{}", row.id.as_str()));
    let shown = (shown_icon(row.mark.as_ref()), letter_of(row));
    let id = row.id.clone();
    let marked = div()
        .relative()
        .size(size)
        .group(GROUP)
        .debug_selector(|| "repo-mark".into())
        .child(mark)
        .child(edit_pencil(size));
    let popover = Popover::new(ElementId::from(key), marked, move |window, cx| {
        let editor = keyed_editor((window, cx), (&sidebar, &id));
        editor.update(cx, |editor, _| editor.show(shown.clone()));
        editor
    })
    .placement(Placement::Right);
    div()
        .on_mouse_down(MouseButton::Left, |_, _, cx| cx.stop_propagation())
        .child(popover)
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use gpui::{Modifiers, TestAppContext};

    use super::*;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn pressing_a_repo_mark_opens_its_popover_without_folding_and_an_emoji_sets_the_repo_icon(
        cx: &mut TestAppContext
    ) {
        let mut floating = crate::test_support::open_floating(cx, DemoShell::reference());
        let mark = floating.window.debug_bounds("repo-mark");
        assert!(
            mark.is_some(),
            "the repo header's mark is the popover's trigger"
        );
        let mark = mark.unwrap_or_default();
        floating
            .window
            .simulate_click(mark.center(), Modifiers::none());
        floating.window.run_until_parked();
        assert!(floating.window.debug_bounds("icon-editor").is_some());
        let folded = floating
            .sidebar
            .read_with(&floating.window, |this, _| this.tree.folded().count());
        assert_eq!(folded, 0, "the press does not fold the repo");
        let actions = floating
            .window
            .debug_bounds("icon-actions")
            .unwrap_or_default();
        floating
            .window
            .simulate_click(actions.center(), Modifiers::none());
        floating.window.simulate_input("⭐");
        floating.window.simulate_keystrokes("enter");
        floating.window.run_until_parked();
        let want = Command::SetRepoIcon {
            row: RowId::from("demo"),
            icon: SpaceIcon::Emoji("⭐".into()),
        };
        assert_eq!(floating.recorder.sent(), vec![want]);
    }

    #[gpui::test]
    fn a_repo_popover_takes_no_press_while_a_request_for_its_icon_runs(cx: &mut TestAppContext) {
        let busy = IconTarget::Repo(RowId::from("demo"));
        let data = DemoShell::reference().with_busy_icon(busy);
        let mut floating = crate::test_support::open_floating(cx, data);
        let mark = floating
            .window
            .debug_bounds("repo-mark")
            .unwrap_or_default();
        floating
            .window
            .simulate_click(mark.center(), Modifiers::none());
        floating.window.run_until_parked();
        let actions = floating
            .window
            .debug_bounds("icon-actions")
            .unwrap_or_default();
        let github = point(actions.right() - px(4.0), actions.center().y);
        for press in [actions.center(), github] {
            floating.window.simulate_click(press, Modifiers::none());
        }
        floating.window.simulate_input("⭐");
        floating.window.simulate_keystrokes("enter");
        floating.window.run_until_parked();
        assert_eq!(
            floating.recorder.sent(),
            Vec::new(),
            "Emoji and GitHub are disabled, as develop's while loading"
        );
    }

    #[test]
    fn the_popover_previews_the_rows_icon_over_its_letter_tile() {
        let mut row = Row::new("r", sidebar::RowKind::RepoHeader, 0, "checkout");
        row.mark = Some(Mark::Emoji("⭐".into()));
        assert_eq!(shown_icon(row.mark.as_ref()), SpaceIcon::Emoji("⭐".into()));
        assert_eq!(
            letter_of(&row),
            Mark::Letter {
                letter: 'C',
                tint: AvatarTint::default()
            }
        );
        row.mark = Some(Mark::Image(PathBuf::from("/i.png")));
        assert_eq!(
            shown_icon(row.mark.as_ref()),
            SpaceIcon::Image("/i.png".into())
        );
    }
}
