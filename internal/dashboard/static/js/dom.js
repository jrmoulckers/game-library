// dom.js: small DOM-construction and formatting helpers shared by every
// section module. Nothing here talks to the network or duplicates any
// domain rule from the Go packages; it only builds/updates DOM nodes from
// data those packages already computed.

// el builds one DOM element. attrs may set "class"/"text" specially, wire
// up "onclick"-style handlers via addEventListener, and otherwise sets a
// plain attribute. Text content is always set via textContent (never
// innerHTML), so nothing rendered through this helper can ever inject
// markup, independent of the page's Content-Security-Policy.
export function el(tag, attrs = {}, children = []) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (value === undefined || value === null) continue;
    if (key === "class") {
      node.className = value;
    } else if (key === "text") {
      node.textContent = value;
    } else if (key.startsWith("on") && typeof value === "function") {
      node.addEventListener(key.slice(2), value);
    } else if (key === "checked") {
      node.checked = Boolean(value);
    } else if (key === "disabled") {
      node.disabled = Boolean(value);
    } else {
      node.setAttribute(key, value);
    }
  }
  for (const child of [].concat(children)) {
    if (child === undefined || child === null || child === false) continue;
    node.appendChild(typeof child === "string" ? document.createTextNode(child) : child);
  }
  return node;
}

export function clear(node) {
  while (node.firstChild) node.removeChild(node.firstChild);
}

export function replaceChildren(node, children) {
  clear(node);
  for (const child of [].concat(children)) {
    if (child === undefined || child === null || child === false) continue;
    node.appendChild(typeof child === "string" ? document.createTextNode(child) : child);
  }
}

const BYTE_UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];

export function formatBytes(value) {
  if (typeof value !== "number" || Number.isNaN(value)) return "unknown";
  let amount = value;
  let unit = 0;
  while (amount >= 1024 && unit < BYTE_UNITS.length - 1) {
    amount /= 1024;
    unit += 1;
  }
  const rounded = unit === 0 ? String(amount) : amount.toFixed(1);
  return `${rounded} ${BYTE_UNITS[unit]}`;
}

export function formatDateTime(iso) {
  if (!iso) return "unknown";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString(undefined, { hour12: false });
}

export function formatAgo(seconds) {
  if (typeof seconds !== "number" || Number.isNaN(seconds)) return "unknown age";
  if (seconds < 0) return "just now";
  if (seconds < 60) return `${Math.floor(seconds)}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
}

export function yesNo(value) {
  return value ? "Yes" : "No";
}

// emptyRow renders a single full-width table row with an explanatory
// message, used for a table's explicit empty state (never a blank table).
export function emptyRow(columnCount, message) {
  return el("tr", {}, [el("td", { colspan: String(columnCount), class: "empty-row", text: message })]);
}
