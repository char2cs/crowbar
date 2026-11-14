/** The tab label: the page title, else the host, else a placeholder. */
export function browserTabName(title: string | undefined, url: string): string {
  if (title?.trim()) return title
  if (url === '') return 'Browser'
  try {
    return new URL(url).host || url
  } catch {
    return url
  }
}
