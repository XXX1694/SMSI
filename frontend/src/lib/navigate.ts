/** A full-page navigation to another origin (a provider's consent page). A seam so tests can observe it. */
export function navigateTo(url: string): void {
  window.location.assign(url);
}
