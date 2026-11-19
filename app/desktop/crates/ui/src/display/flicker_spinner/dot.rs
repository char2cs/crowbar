//! One dot of a sprite: where it sits and its opacity in each frame.

use crate::display::flicker_spinner::markup::attr;

pub(crate) struct Dot {
    x: f32,
    y: f32,
    radius: f32,
    opacities: Vec<f32>,
}

impl Dot {
    pub(crate) fn parse(circle: &str) -> Option<Self> {
        let (tag, _) = circle.split_once('>')?;
        let opacities = attr(circle, "values")?
            .split(';')
            .map(str::parse)
            .collect::<Result<Vec<f32>, _>>()
            .ok()?;
        Some(Self {
            x: attr(tag, "cx")?.parse().ok()?,
            y: attr(tag, "cy")?.parse().ok()?,
            radius: attr(tag, "r")?.parse().ok()?,
            opacities,
        })
    }

    pub(crate) fn frames(&self) -> u32 {
        u32::try_from(self.opacities.len()).unwrap_or(0)
    }

    pub(crate) fn opacity(
        &self,
        frame: usize,
    ) -> f32 {
        self.opacities.get(frame).copied().unwrap_or(0.0)
    }

    pub(crate) fn left(&self) -> f32 {
        self.x - self.radius
    }

    pub(crate) fn top(&self) -> f32 {
        self.y - self.radius
    }

    pub(crate) fn diameter(&self) -> f32 {
        self.radius * 2.0
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CIRCLE: &str = r#" cx="9" cy="3" r="2" fill-opacity="1"><animate attributeName="fill-opacity" values="1;0.15;0.5" dur="0.27s"/></circle>"#;

    #[test]
    fn a_dot_reads_its_box_and_one_opacity_per_frame() {
        let dot = Dot::parse(CIRCLE).expect("parses");
        assert_eq!((dot.left(), dot.top(), dot.diameter()), (7.0, 1.0, 4.0));
        assert_eq!(dot.frames(), 3);
        assert_eq!(dot.opacity(1), 0.15);
        assert_eq!(dot.opacity(9), 0.0, "past the timeline");
    }
}
