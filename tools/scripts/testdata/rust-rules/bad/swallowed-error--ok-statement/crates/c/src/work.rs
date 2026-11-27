/// Works.
pub fn work(text: &str) {
    text.parse::<u32>().ok();
}

#[cfg(test)]
mod tests {}
