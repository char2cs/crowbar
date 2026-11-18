//! Messages a terminal view sends to the daemon.

use serde_json::json;

/// One client-to-daemon text message of the PTY websocket.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ClientMessage {
    /// Keystrokes or pasted text for the program.
    Input(String),
    /// The grid changed size.
    Resize {
        /// Columns.
        columns: u16,
        /// Rows.
        rows: u16,
    },
    /// The host theme, so the daemon can answer colour queries and DEC 2031 reports.
    Theme {
        /// Opaque background as `#rrggbb`.
        background: String,
        /// Foreground as `#rrggbb`.
        foreground: String,
        /// Whether the theme is dark.
        dark: bool,
    },
}

impl ClientMessage {
    /// The JSON text frame for this message.
    pub fn to_json(&self) -> String {
        self.payload().to_string()
    }

    fn payload(&self) -> serde_json::Value {
        match self {
            Self::Input(data) => json!({ "data": data }),
            Self::Resize { columns, rows } => {
                json!({ "type": "resize", "cols": columns, "rows": rows })
            },
            Self::Theme {
                background,
                foreground,
                dark,
            } => {
                json!({ "type": "theme", "bg": background, "fg": foreground, "dark": dark })
            },
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn messages_use_the_daemon_field_names() {
        let cases = [
            (ClientMessage::Input("ls\r".into()), r#"{"data":"ls\r"}"#),
            (
                ClientMessage::Resize {
                    columns: 80,
                    rows: 24,
                },
                r#"{"cols":80,"rows":24,"type":"resize"}"#,
            ),
            (
                ClientMessage::Theme {
                    background: "#000000".into(),
                    foreground: "#ffffff".into(),
                    dark: true,
                },
                r##"{"bg":"#000000","dark":true,"fg":"#ffffff","type":"theme"}"##,
            ),
        ];
        for (message, expected) in cases {
            let sent: serde_json::Value =
                serde_json::from_str(&message.to_json()).unwrap_or_default();
            let wanted: serde_json::Value = serde_json::from_str(expected).unwrap_or_default();
            assert_eq!(sent, wanted);
        }
    }
}
