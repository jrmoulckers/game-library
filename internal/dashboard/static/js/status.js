// status.js: the page's two ARIA live regions. Every section module
// reports through announceStatus/announceError instead of writing its own
// alerts, so a screen-reader user gets one consistent, predictable place
// to hear what happened.

let statusRegion = null;
let errorRegion = null;

export function initStatus() {
  statusRegion = document.getElementById("status-region");
  errorRegion = document.getElementById("error-region");
}

export function announceStatus(message) {
  if (statusRegion) statusRegion.textContent = message;
}

export function announceError(message) {
  if (errorRegion) errorRegion.textContent = message;
}

export function clearError() {
  if (errorRegion) errorRegion.textContent = "";
}
