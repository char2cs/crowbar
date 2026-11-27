use gpui::prelude::FluentBuilder;
use gpui::{Div, ParentElement, SharedString, Styled, div, px};
use sidebar::{ChangedRow, Command, Intent};
use ui::{Checkbox, Pose, Surface};

use crate::sidebar::{RowAction, RowHandler};

const CONTROL_BOX: f32 = 16.0;

fn marks(
    row: &ChangedRow,
    asked: Pose,
) -> Vec<(String, Option<String>)> {
    let on = asked == Pose::On;
    match row {
        ChangedRow::File { file, .. } => {
            vec![(file.path.clone(), on.then(|| file.signature.clone()))]
        },
        ChangedRow::Folder { files, .. } => files
            .iter()
            .map(|(path, signature)| (path.clone(), on.then(|| signature.clone())))
            .collect(),
    }
}

fn shown(row: &ChangedRow) -> (bool, bool) {
    match row {
        ChangedRow::File { viewed, .. } => (*viewed, false),
        ChangedRow::Folder { files, viewed, .. } => {
            let all = !files.is_empty() && *viewed == files.len();
            (all, *viewed > 0 && !all)
        },
    }
}

fn path_of(row: &ChangedRow) -> &str {
    match row {
        ChangedRow::File { file, .. } => &file.path,
        ChangedRow::Folder { path, .. } => path,
    }
}

pub(crate) fn viewed_box(
    row: &ChangedRow,
    handler: &RowHandler,
) -> Div {
    let (checked, mixed) = shown(row);
    let id = SharedString::from(format!("viewed-{}", path_of(row)));
    let (row, handler) = (row.clone(), std::rc::Rc::clone(handler));
    let checkbox =
        Checkbox::new(id, checked)
            .on(Surface::Chrome)
            .on_change(move |asked, window, cx| {
                cx.stop_propagation();
                let files = marks(&row, asked);
                let intent = Intent::Send(Command::SetFilesViewed { files });
                handler(RowAction::Intent(intent), window, cx);
            });
    div()
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .w(px(CONTROL_BOX))
        .child(checkbox.when(mixed, Checkbox::indeterminate))
}

#[cfg(test)]
mod tests {
    use sidebar::{ChangedFile, Expansion};

    use super::*;

    fn folder(viewed: usize) -> ChangedRow {
        ChangedRow::Folder {
            path: "src".into(),
            label: "src".into(),
            depth: 0,
            expansion: Expansion::Expanded,
            files: vec![
                ("src/a.ts".into(), "A||1|0".into()),
                ("src/b.ts".into(), "M||2|1".into()),
            ],
            viewed,
        }
    }

    #[test]
    fn a_folder_tick_marks_every_file_under_it_and_untick_clears_them() {
        let on = marks(&folder(1), Pose::On);
        let off = marks(&folder(2), Pose::Off);
        assert_eq!(
            on,
            [
                ("src/a.ts".to_owned(), Some("A||1|0".to_owned())),
                ("src/b.ts".to_owned(), Some("M||2|1".to_owned()))
            ]
        );
        assert!(off.iter().all(|(_, signature)| signature.is_none()));
        assert_eq!(
            [shown(&folder(0)), shown(&folder(1)), shown(&folder(2))],
            [(false, false), (false, true), (true, false)]
        );
    }

    #[test]
    fn a_file_tick_carries_its_current_signature() {
        let changed = ChangedFile {
            path: "a.ts".into(),
            signature: "M||1|0".into(),
            ..ChangedFile::default()
        };
        let file = ChangedRow::File {
            file: changed,
            name: "a.ts".into(),
            depth: 0,
            threads: 0,
            viewed: false,
        };
        assert_eq!(
            marks(&file, Pose::On),
            [("a.ts".to_owned(), Some("M||1|0".to_owned()))]
        );
    }
}
