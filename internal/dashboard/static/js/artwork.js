// artwork.js: the server-paginated artwork/media browser with filters.
// Every filter and every row comes straight from
// GET /api/review/observations; this module only renders it and manages
// the current page number client-side.

import { el, clear, emptyRow } from "./dom.js";

let currentPage = 1;
let pageSize = 24;
let currentFilters = {};
let lastTotal = 0;

function buildQuery(filters, page) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (value) params.set(key, value);
  }
  params.set("page", String(page));
  params.set("pageSize", String(pageSize));
  return params.toString();
}

function dimensionKey(media) {
  if (!media || !media.width || !media.height) return "\u2014";
  return `${media.width}x${media.height}`;
}

function renderRow(entry) {
  const obs = entry.observation;
  const label = obs.identityHint || obs.relativePath;
  const isImage = (obs.media && obs.media.mime || "").startsWith("image/");
  let preview;
  if (isImage) {
    const img = el("img", {
      class: "thumb",
      src: `/api/review/media/${encodeURIComponent(entry.id)}`,
      alt: `${label}, role ${obs.media.role || "unknown"}, ${dimensionKey(obs.media)}`,
    });
    img.addEventListener("error", () => {
      preview.replaceWith(el("span", { class: "thumb-broken", text: "Preview unavailable" }));
    });
    preview = img;
  } else {
    preview = el("span", { class: "thumb-broken", text: obs.media && obs.media.mime ? obs.media.mime : "No preview" });
  }

  const themes = entry.themes && entry.themes.length ? entry.themes.join(", ") : "\u2014";
  const policy = entry.policyMode
    ? `${entry.policyMode}${entry.policyRuleMatched ? ` (rule ${entry.policyRuleIndex})` : " (default)"}`
    : "\u2014";
  const issueCount = (entry.validationIssues || []).length;
  const state = issueCount > 0 ? `${issueCount} issue(s)` : "Clean";

  return el("tr", {}, [
    el("td", {}, [preview]),
    el("td", { text: label }),
    el("td", { text: obs.rootId }),
    el("td", { text: obs.system || "\u2014" }),
    el("td", { text: (obs.media && obs.media.role) || "\u2014" }),
    el("td", { text: dimensionKey(obs.media) }),
    el("td", { text: policy }),
    el("td", { text: themes }),
    el("td", { text: state, class: issueCount > 0 ? "state-warn" : "state-ok" }),
  ]);
}

export async function init(root, { api, announceStatus, announceError }) {
  const form = root.querySelector("#artwork-filters");
  const resetButton = root.querySelector("#artwork-reset");
  const summary = root.querySelector("#artwork-summary");
  const tbody = root.querySelector("#artwork-body");
  const prevButton = root.querySelector("#artwork-prev");
  const nextButton = root.querySelector("#artwork-next");
  const pageStatus = root.querySelector("#artwork-page-status");

  async function load() {
    try {
      const page = await api.get(`/api/review/observations?${buildQuery(currentFilters, currentPage)}`);
      lastTotal = page.total || 0;
      clear(tbody);
      if (!page.items || page.items.length === 0) {
        tbody.appendChild(emptyRow(9, "No artwork matches the current filters."));
      } else {
        for (const entry of page.items) tbody.appendChild(renderRow(entry));
      }
      const totalPages = Math.max(1, Math.ceil(lastTotal / pageSize));
      summary.textContent = `${lastTotal} matching observation(s).`;
      pageStatus.textContent = `Page ${currentPage} of ${totalPages}`;
      prevButton.disabled = currentPage <= 1;
      nextButton.disabled = currentPage >= totalPages;
    } catch (err) {
      if (err.code === "config_missing") {
        summary.textContent = "No active configuration saved yet. Complete first-run setup, then return here.";
      } else {
        announceError(`Artwork browser: failed to load observations (${err.message}).`);
      }
    }
  }

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const data = new FormData(form);
    currentFilters = Object.fromEntries(data.entries());
    currentPage = 1;
    load();
  });

  resetButton.addEventListener("click", () => {
    form.reset();
    currentFilters = {};
    currentPage = 1;
    load();
  });

  prevButton.addEventListener("click", () => {
    if (currentPage > 1) {
      currentPage -= 1;
      load();
    }
  });

  nextButton.addEventListener("click", () => {
    const totalPages = Math.max(1, Math.ceil(lastTotal / pageSize));
    if (currentPage < totalPages) {
      currentPage += 1;
      load();
    }
  });

  await load();
}
