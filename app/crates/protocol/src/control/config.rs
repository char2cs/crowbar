//! What the shell pushes about the user's setup: setting keys and values, theme tokens and key bindings.

mod key_binding;
mod key_chord;
mod pane_setting_keys;
mod precedence;
mod setting_key;
mod setting_values;
mod theme_tokens;

pub use key_binding::KeyBinding;
pub use key_chord::KeyChord;
pub use pane_setting_keys::PANE_SETTING_KEYS;
pub use precedence::Precedence;
pub use setting_key::SettingKey;
pub use setting_values::{MAX_SETTING_VALUES, SETTINGS_FRAME_VERSION, SettingValues};
pub use theme_tokens::{MAX_THEME_TOKENS, ThemeTokens};
