//! Command line of the demo.

use ui::ThemeMode;

const DEFAULT_WIDTH: f32 = 1000.0;
const DEFAULT_HEIGHT: f32 = 640.0;

#[derive(Debug, Clone)]
pub(crate) struct Arguments {
    pub(crate) mode: ThemeMode,
    pub(crate) command: String,
    pub(crate) width: f32,
    pub(crate) height: f32,
}

impl Arguments {
    pub(crate) fn from_env() -> Self {
        let mut arguments = Self {
            mode: ThemeMode::Dark,
            command: String::new(),
            width: DEFAULT_WIDTH,
            height: DEFAULT_HEIGHT,
        };
        let mut words = std::env::args().skip(1);
        while let Some(flag) = words.next() {
            let value = words.next().unwrap_or_default();
            arguments.set(&flag, &value);
        }
        arguments
    }

    fn set(
        &mut self,
        flag: &str,
        value: &str,
    ) {
        match flag {
            "--mode" => self.mode = mode_of(value),
            "--type" => value.clone_into(&mut self.command),
            "--size" => self.set_size(value),
            _ => {},
        }
    }

    fn set_size(
        &mut self,
        value: &str,
    ) {
        let Some((width, height)) = value.split_once('x') else {
            return;
        };
        self.width = width.parse().unwrap_or(DEFAULT_WIDTH);
        self.height = height.parse().unwrap_or(DEFAULT_HEIGHT);
    }
}

fn mode_of(value: &str) -> ThemeMode {
    match value {
        "light" => ThemeMode::Light,
        _ => ThemeMode::Dark,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn flags_set_their_fields() {
        let mut arguments = Arguments {
            mode: ThemeMode::Dark,
            command: String::new(),
            width: 1.0,
            height: 1.0,
        };
        arguments.set("--mode", "light");
        arguments.set("--type", "ls");
        arguments.set("--size", "800x600");
        arguments.set("--size", "bad");
        assert_eq!(arguments.mode, ThemeMode::Light);
        assert_eq!(arguments.command, "ls");
        assert_eq!((arguments.width, arguments.height), (800.0, 600.0));
    }
}
