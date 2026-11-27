use std::time::Duration;

use crate::display::flicker_spinner::dot::Dot;
use crate::display::flicker_spinner::markup::attr;

const MILLIS_PER_SECOND: u64 = 1000;
const FRACTION_DIGITS: usize = 3;

pub(crate) struct Sprite {
    width: f32,
    frames: u32,
    step: Duration,
    dots: Vec<Dot>,
}

impl Sprite {
    pub(crate) fn parse(source: &str) -> Option<Self> {
        let width = view_width(attr(source, "viewBox")?)?;
        let dots: Vec<Dot> = source
            .split("<circle")
            .skip(1)
            .filter_map(Dot::parse)
            .collect();
        let first = source.split("<animate").nth(1)?;
        let frames = attr(first, "values")?.split(';').count();
        let frames = u32::try_from(frames).ok().filter(|frames| *frames > 0)?;
        let cycle = Duration::from_millis(millis(attr(first, "dur")?)?);
        let even = dots.iter().all(|dot| dot.frames() == frames);
        even.then_some(Self {
            width,
            frames,
            step: cycle / frames,
            dots,
        })
    }

    pub(crate) fn width(&self) -> f32 {
        self.width
    }

    pub(crate) fn dots(&self) -> &[Dot] {
        &self.dots
    }

    pub(crate) fn frame_at(
        &self,
        elapsed: Duration,
    ) -> (usize, Duration) {
        let step = self.step.as_nanos().max(1);
        let steps = elapsed.as_nanos() / step;
        let frame = usize::try_from(steps % u128::from(self.frames)).unwrap_or_default();
        let into = Duration::from_nanos(u64::try_from(elapsed.as_nanos() % step).unwrap_or(0));
        (frame, self.step.saturating_sub(into))
    }
}

fn view_width(view_box: &str) -> Option<f32> {
    view_box.split_whitespace().nth(2)?.parse().ok()
}

fn millis(seconds: &str) -> Option<u64> {
    let seconds = seconds.strip_suffix('s')?;
    let (whole, fraction) = seconds.split_once('.').unwrap_or((seconds, ""));
    if fraction.len() > FRACTION_DIGITS {
        return None;
    }
    let fraction: u64 = format!("{fraction:0<FRACTION_DIGITS$}").parse().ok()?;
    Some(whole.parse::<u64>().ok()? * MILLIS_PER_SECOND + fraction)
}

#[cfg(test)]
mod tests {
    use super::*;

    const TWO_FRAMES: &str = r#"<svg viewBox="0 0 30 30"><circle cx="3" cy="3" r="2" fill-opacity="0.15"><animate attributeName="fill-opacity" values="0.15;1" dur="0.18s" calcMode="discrete" repeatCount="indefinite"/></circle><circle cx="9" cy="3" r="2" fill-opacity="1"><animate attributeName="fill-opacity" values="1;0.15" dur="0.18s" calcMode="discrete" repeatCount="indefinite"/></circle></svg>"#;

    type Case = (u64, usize, u64);

    const CASES: &[Case] = &[
        (0, 0, 90),
        (89, 0, 1),
        (90, 1, 90),
        (180, 0, 90),
        (200, 0, 70),
    ];

    #[test]
    fn a_sprite_reads_its_dots_frames_and_step() {
        let sprite = Sprite::parse(TWO_FRAMES).expect("parses");
        assert_eq!(sprite.width(), 30.0);
        assert_eq!(sprite.dots().len(), 2);
        assert_eq!(sprite.frame_at(Duration::ZERO).1, Duration::from_millis(90));
    }

    #[test]
    fn frames_step_at_their_edges_and_loop() {
        let sprite = Sprite::parse(TWO_FRAMES).expect("parses");
        for &(at, frame, left) in CASES {
            let got = sprite.frame_at(Duration::from_millis(at));
            assert_eq!(got, (frame, Duration::from_millis(left)), "at {at} ms");
        }
    }

    #[test]
    fn durations_are_read_in_whole_milliseconds() {
        const DURATIONS: &[(&str, Option<u64>)] = &[
            ("0.72s", Some(720)),
            ("1.5s", Some(1500)),
            ("2s", Some(2000)),
            ("0.0001s", None),
            ("90ms", None),
        ];
        for &(text, want) in DURATIONS {
            assert_eq!(millis(text), want, "{text}");
        }
    }
}
