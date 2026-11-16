use model::Command;

#[derive(Debug, Clone, PartialEq)]
pub(crate) enum Closing {
    Panes(Vec<Command>),
    Window,
}
