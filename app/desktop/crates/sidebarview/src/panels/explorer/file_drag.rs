//! Moving Files rows by dragging them onto a folder, beside a file or onto the panel itself,
//! as develop's `use-file-explorer-drag-drop.ts` does; a file dropped on a pane opens there.

mod drop_area;
mod file_drag_state;
mod file_draggable;
mod file_ghost;

pub(crate) use file_drag_state::FileDragState;
pub(crate) use file_draggable::file_draggable;

use std::time::Duration;

use gpui::{Context, Pixels, Point, Window, px};
use sidebar::{Command, DropSpot, FileDrop};

use crate::sidebar::{FileGesture, Sidebar};

// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop.ts
const UNFOLD_AFTER: Duration = Duration::from_millis(550);

fn xy(at: Point<Pixels>) -> (f32, f32) {
    (at.x.into(), at.y.into())
}

impl Sidebar {
    pub(crate) fn file_gesture(
        &mut self,
        gesture: FileGesture,
        cx: &mut Context<'_, Self>,
    ) {
        match gesture {
            FileGesture::Press(row, at) => self.file_drag = FileDragState::pressed(row, xy(at)),
            FileGesture::Over(spot, at) => self.file_over(spot, xy(at), cx),
            FileGesture::Left(spot) => self.file_left(&spot, cx),
            FileGesture::Roam(at) => self.file_roam(xy(at), cx),
            FileGesture::Away => self.file_left(&DropSpot::Root, cx),
            FileGesture::Edge(pull) => self.pull_files_edge(pull, cx),
            FileGesture::Dropped => self.drop_file(cx),
        }
    }

    fn file_over(
        &mut self,
        spot: DropSpot,
        at: (f32, f32),
        cx: &mut Context<'_, Self>,
    ) {
        if self.file_drag.lift(at) && self.file_drag.hover(spot) {
            self.aim_unfold(cx);
            cx.notify();
        }
    }

    fn file_roam(
        &mut self,
        at: (f32, f32),
        cx: &mut Context<'_, Self>,
    ) {
        let off_rows = self.file_drag.lift(at) && self.file_drag.over.is_none();
        if off_rows && self.file_drag.hover(DropSpot::Root) {
            self.aim_unfold(cx);
            cx.notify();
        }
    }

    fn file_left(
        &mut self,
        spot: &DropSpot,
        cx: &mut Context<'_, Self>,
    ) {
        if self.file_drag.leave(spot) {
            self.file_drag.unfolding = None;
            cx.notify();
        }
    }

    fn aim_unfold(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let folded = self.file_drag.target();
        let Some(path) = folded
            .as_ref()
            .and_then(FileDrop::unfolds)
            .map(str::to_owned)
        else {
            self.file_drag.unfolding = None;
            return;
        };
        let unfolding = cx.spawn(async move |this, cx| {
            cx.background_executor().timer(UNFOLD_AFTER).await;
            this.update(cx, |this, cx| this.unfold(path, cx))
        });
        self.file_drag.unfolding = Some(unfolding);
    }

    fn unfold(
        &mut self,
        path: String,
        cx: &mut Context<'_, Self>,
    ) {
        let target = self.file_drag.target();
        if target.as_ref().and_then(FileDrop::unfolds) == Some(path.as_str()) {
            self.send(Command::ToggleDirectory(path), cx);
        }
    }

    fn pull_files_edge(
        &mut self,
        pull: f32,
        cx: &mut Context<'_, Self>,
    ) {
        if (pull - self.file_drag.pull).abs() < f32::EPSILON {
            return;
        }
        self.file_drag.pull = pull;
        cx.notify();
    }

    pub(crate) fn scroll_files_edge(
        &mut self,
        window: &mut Window,
    ) {
        if self.file_drag.pull == 0.0 || self.file_drag.lifted.is_none() {
            return;
        }
        let mut offset = self.scrolls.files.offset();
        offset.y -= px(self.file_drag.pull);
        self.scrolls.files.set_offset(offset);
        window.request_animation_frame();
    }

    fn drop_file(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let lifted = self.file_drag.lifted.clone();
        let moved = lifted.zip(self.file_drag.target());
        self.end_file_drag(cx);
        let command = moved.and_then(|(row, target)| row.command(&target));
        if let Some(command) = command {
            self.send(command, cx);
        }
    }

    pub(crate) fn end_file_drag(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.file_drag.end();
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{Expansion, FileKind, FileMove};

    use super::*;
    use crate::sidebar::RowAction;
    use crate::test_support::prelude::*;

    const FOLDED: FileKind = FileKind::Directory(Expansion::Collapsed);
    const OPEN: FileKind = FileKind::Directory(Expansion::Expanded);

    fn row(
        path: &str,
        kind: FileKind,
    ) -> DropSpot {
        DropSpot::Row {
            path: path.into(),
            kind,
        }
    }

    fn moved(
        path: &str,
        new_path: &str,
    ) -> Command {
        Command::MoveFile {
            path: path.into(),
            new_path: new_path.into(),
        }
    }

    fn act(
        sidebar: &gpui::Entity<Sidebar>,
        gestures: Vec<FileGesture>,
        cx: &mut gpui::VisualTestContext,
    ) {
        for gesture in gestures {
            let action = RowAction::File(gesture);
            sidebar.update_in(cx, |this, window, cx| this.act(action, window, cx));
        }
    }

    fn lifted_over(
        dragged: FileMove,
        over: FileGesture,
    ) -> Vec<FileGesture> {
        vec![FileGesture::Press(dragged, point(px(0.0), px(0.0))), over]
    }

    type MoveCase = (FileMove, FileGesture, Option<Command>);

    fn move_cases() -> [MoveCase; 5] {
        let lifted = point(px(0.0), px(20.0));
        let readme = || FileMove::new("README.md", FileKind::File);
        let main = || FileMove::new("src/main.rs", FileKind::File);
        [
            (
                readme(),
                FileGesture::Over(row("src", OPEN), lifted),
                Some(moved("README.md", "src/README.md")),
            ),
            (
                main(),
                FileGesture::Roam(lifted),
                Some(moved("src/main.rs", "main.rs")),
            ),
            (
                main(),
                FileGesture::Over(row("src/lib.rs", FileKind::File), lifted),
                None,
            ),
            (
                FileMove::new("src", OPEN),
                FileGesture::Over(row("src/main.rs", FileKind::File), lifted),
                None,
            ),
            (
                readme(),
                FileGesture::Over(row("src", OPEN), point(px(2.0), px(2.0))),
                None,
            ),
        ]
    }

    #[gpui::test]
    fn a_dropped_row_moves_into_the_folder_under_it_or_to_the_root(cx: &mut TestAppContext) {
        let cases = move_cases();
        for (dragged, over, want) in cases {
            let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
            let name = format!("{} with {over:?}", dragged.path());
            let mut gestures = lifted_over(dragged, over);
            gestures.push(FileGesture::Dropped);
            act(&sidebar, gestures, cx);
            let sent: Vec<Command> = recorder
                .sent()
                .into_iter()
                .filter(|c| matches!(c, Command::MoveFile { .. }))
                .collect();
            assert_eq!(sent, Vec::from_iter(want), "{name}");
            assert!(
                !sidebar.read_with(cx, |this, _| this.file_drag.is_live()),
                "{name}: ended"
            );
        }
    }

    #[gpui::test]
    fn the_folder_under_the_pointer_is_drawn_as_the_target_and_the_panel_with_a_dashed_outline(
        cx: &mut TestAppContext
    ) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        let lifted = point(px(0.0), px(20.0));
        let over_src = lifted_over(
            FileMove::new("README.md", FileKind::File),
            FileGesture::Over(row("src", OPEN), lifted),
        );
        act(&sidebar, over_src, cx);
        cx.run_until_parked();
        assert!(
            cx.debug_bounds("file-drop-target").is_some(),
            "src is marked"
        );
        act(
            &sidebar,
            vec![
                FileGesture::Left(row("src", OPEN)),
                FileGesture::Roam(lifted),
            ],
            cx,
        );
        cx.run_until_parked();
        assert!(
            cx.debug_bounds("file-drop-target").is_none(),
            "no row is marked"
        );
        let root = sidebar.read_with(cx, |this, _| {
            this.file_drag.target().is_some_and(|t| t.is_root())
        });
        assert!(root, "the panel is the target");
    }

    #[gpui::test]
    fn escape_cancels_the_drag_and_a_release_then_moves_nothing(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        cx.run_until_parked();
        let lifted = point(px(0.0), px(20.0));
        act(
            &sidebar,
            lifted_over(
                FileMove::new("README.md", FileKind::File),
                FileGesture::Over(row("src", OPEN), lifted),
            ),
            cx,
        );
        sidebar.update_in(cx, |this, window, cx| window.focus(&this.files_focus, cx));
        cx.simulate_keystrokes("escape");
        assert!(
            !sidebar.read_with(cx, |this, _| this.file_drag.is_live()),
            "cancelled"
        );
        act(&sidebar, vec![FileGesture::Dropped], cx);
        assert!(
            !recorder
                .sent()
                .iter()
                .any(|c| matches!(c, Command::MoveFile { .. }))
        );
    }

    #[gpui::test]
    fn a_folded_folder_unfolds_once_the_pointer_rests_on_it(cx: &mut TestAppContext) {
        let lifted = point(px(0.0), px(20.0));
        let resting = UNFOLD_AFTER;
        let cases = [(resting, true), (resting / 2, false)];
        for (rest, unfolds) in cases {
            let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
            let over = FileGesture::Over(row("lib", FOLDED), lifted);
            act(
                &sidebar,
                lifted_over(FileMove::new("README.md", FileKind::File), over),
                cx,
            );
            cx.executor().advance_clock(rest);
            act(&sidebar, vec![FileGesture::Left(row("lib", FOLDED))], cx);
            cx.executor().advance_clock(resting);
            cx.run_until_parked();
            let toggled = recorder
                .sent()
                .contains(&Command::ToggleDirectory("lib".into()));
            assert_eq!(toggled, unfolds, "rested {rest:?}");
        }
    }
}
