use std::ops::Range;
use std::time::Instant;

use gpui::{Hsla, PaintQuad, ShapedLine, TextRun, UnderlineStyle, px};

use crate::control::editor::field::frame::Frame;
use crate::foundation::Keyframes;
use crate::foundation::Motion;
use crate::foundation::tokens::timing;

pub(crate) fn caret_frame(
    line: ShapedLine,
    caret: PaintQuad,
) -> Frame {
    Frame {
        line,
        selection: None,
        caret: Some(caret),
    }
}

pub(crate) fn band_frame(
    line: ShapedLine,
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

pub(crate) fn caret_color(
    color: Hsla,
    since: Instant,
    motion: Motion,
) -> Hsla {
    if motion.is_reduced() {
        return color;
    }
    let cycle = timing::CARET.as_secs_f32();
    let phase = (since.elapsed().as_secs_f32() % cycle) / cycle;
    color.opacity(Keyframes::CARET_BLINK.sample(phase))
}

#[cfg(test)]
mod tests {
    use std::ops::Range;
    use std::time::Instant;

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

    #[test]
    fn caret_is_solid_when_motion_is_reduced_and_visible_at_the_start_of_a_blink() {
        let colour = hsla(0.5, 0.5, 0.5, 1.0);
        assert_eq!(
            caret_color(colour, Instant::now(), Motion::reduced()),
            colour
        );
        assert!(caret_color(colour, Instant::now(), Motion::default()).a > 0.9);
    }
}
