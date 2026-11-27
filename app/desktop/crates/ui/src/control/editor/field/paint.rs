use std::ops::Range;

use gpui::{PaintQuad, TextRun, UnderlineStyle, px};

use crate::control::editor::field::frame::Frame;
use crate::display::TrackedLine;

pub(crate) fn caret_frame(
    line: TrackedLine,
    caret: PaintQuad,
) -> Frame {
    Frame {
        line,
        selection: None,
        caret: Some(caret),
    }
}

pub(crate) fn band_frame(
    line: TrackedLine,
    band: PaintQuad,
) -> Frame {
    Frame {
        line,
        selection: Some(band),
        caret: None,
    }
}

fn plain_run(
    len: usize,
    base: &TextRun,
) -> TextRun {
    TextRun {
        len,
        ..base.clone()
    }
}

pub(crate) fn runs_for(
    len: usize,
    marked: Option<Range<usize>>,
    base: &TextRun,
) -> Vec<TextRun> {
    let Some(marked) = marked else {
        return vec![plain_run(len, base)];
    };
    let underline = UnderlineStyle {
        color: Some(base.color),
        thickness: px(1.0),
        wavy: false,
    };
    [
        TextRun {
            len: marked.start,
            ..base.clone()
        },
        TextRun {
            len: marked.end - marked.start,
            underline: Some(underline),
            ..base.clone()
        },
        TextRun {
            len: len - marked.end,
            ..base.clone()
        },
    ]
    .into_iter()
    .filter(|run| run.len > 0)
    .collect()
}

#[cfg(test)]
mod tests {
    use std::ops::Range;

    use gpui::{TextRun, hsla, px};

    use super::*;

    fn base(len: usize) -> TextRun {
        TextRun {
            len,
            font: gpui::font("Cal Sans UI"),
            color: hsla(0.0, 0.0, 0.0, 1.0),
            background_color: None,
            underline: None,
            strikethrough: None,
        }
    }

    type Runs = &'static [(usize, bool)];

    type Case = (&'static str, usize, Option<Range<usize>>, Runs);

    const CASES: &[Case] = &[
        ("unmarked text is one run", 5, None, &[(5, false)]),
        (
            "marked text is underlined between plain runs",
            6,
            Some(2..4),
            &[(2, false), (2, true), (2, false)],
        ),
        (
            "marks at the edges drop empty runs",
            4,
            Some(0..4),
            &[(4, true)],
        ),
    ];

    #[test]
    fn runs_split_around_the_marked_range() {
        for (name, len, marked, expected) in CASES {
            let runs = runs_for(*len, marked.clone(), &base(*len));
            let got: Vec<(usize, bool)> = runs
                .iter()
                .map(|run| (run.len, run.underline.is_some()))
                .collect();
            assert_eq!(got, *expected, "{name}");
            let thickness = runs
                .iter()
                .filter_map(|run| run.underline)
                .map(|u| u.thickness);
            assert!(
                thickness.into_iter().all(|t| t == px(1.0)),
                "{name}: thickness"
            );
        }
    }
}
