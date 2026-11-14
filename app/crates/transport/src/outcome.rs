use crate::ending::Ending;

pub(crate) enum Outcome {
    Upload(Ending),
    Download(Ending),
    Revoked,
}
