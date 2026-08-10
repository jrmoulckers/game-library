// overview.js: library health, scan freshness, unresolved queues, adapter
// readiness, and an artwork strip sample. Every number here is exactly
// what the Go review.Overview/adapter-status endpoints computed.

import { el, clear, formatBytes, formatAgo, formatDateTime, yesNo, emptyRow } from "./dom.js";

let lastOverview = null;

export function getLastInventoryDigest() {
  return lastOverview ? lastOverview.inventoryDigest || "" : "";
}

function renderFreshness(el_, overview) {
  const age = formatAgo(overview.scanAgeSeconds);
  const when = formatDateTime(overview.scannedAt);
  el_.textContent = `Snapshot source: ${overview.source || "unknown"}. Privacy: ${overview.privacy || "unknown"}. Scanned ${when} (${age}).`;
}

function renderRoots(tbody, overview) {
  clear(tbody);
  const roots = overview.roots || [];
  if (roots.length === 0) {
    tbody.appendChild(emptyRow(8, "No roots configured yet. Visit first-run setup to add one."));
    return;
  }
  for (const r of roots) {
    tbody.appendChild(
      el("tr", {}, [
        el("td", { text: r.rootId }),
        el("td", { text: r.kind }),
        el("td", { text: r.system || "\u2014" }),
        el("td", { text: String(r.fileCount) }),
        el("td", { text: formatBytes(r.totalBytes) }),
        el("td", { text: String(r.mediaCount) }),
        el("td", { text: String(r.imageCount) }),
        el("td", { text: String(r.issueCount), class: r.issueCount > 0 ? "state-warn" : "state-ok" }),
      ]),
    );
  }
}

function renderQueues(list, overview) {
  clear(list);
  const items = [
    `${overview.totalIssues || 0} validation issue(s) to resolve`,
    `${(overview.duplicateSummary && overview.duplicateSummary.groups) || 0} duplicate group(s) awaiting classification`,
  ];
  for (const text of items) {
    list.appendChild(el("li", { text }));
  }
}

function renderDuplicateSummary(dl, overview) {
  clear(dl);
  const s = overview.duplicateSummary || {};
  const rows = [
    ["Groups", String(s.groups || 0)],
    ["Copies", String(s.copies || 0)],
    ["Excess bytes", formatBytes(s.excessBytes || 0)],
    ["Cross-root groups", String(s.crossRootGroups || 0)],
    ["Unique file hashes", String(s.uniqueFileHashes || 0)],
  ];
  for (const [term, value] of rows) {
    dl.appendChild(el("dt", { text: term }));
    dl.appendChild(el("dd", { text: value }));
  }
}

function renderIssues(tbody, overview) {
  clear(tbody);
  const issues = overview.issues || [];
  if (issues.length === 0) {
    tbody.appendChild(emptyRow(4, "No validation issues recorded."));
    return;
  }
  for (const issue of issues) {
    tbody.appendChild(
      el("tr", {}, [
        el("td", { text: issue.rootId || "\u2014" }),
        el("td", { text: issue.relativePath || "\u2014" }),
        el("td", { text: issue.code }),
        el("td", { text: issue.message }),
      ]),
    );
  }
}

function renderAdapters(tbody, adapters) {
  clear(tbody);
  if (!adapters || adapters.length === 0) {
    tbody.appendChild(emptyRow(4, "No adapter status available."));
    return;
  }
  for (const a of adapters) {
    tbody.appendChild(
      el("tr", {}, [
        el("td", { text: a.adapter }),
        el("td", { text: yesNo(a.planOnly) }),
        el("td", { text: yesNo(a.destinationConfigured) }),
        el("td", { text: yesNo(a.inputReady) }),
      ]),
    );
  }
}

function renderArtworkStrip(list, observationPage) {
  clear(list);
  const items = (observationPage && observationPage.items) || [];
  if (items.length === 0) {
    list.appendChild(el("li", { text: "No observations available yet." }));
    return;
  }
  for (const entry of items.slice(0, 8)) {
    const obs = entry.observation;
    const label = obs.identityHint || obs.relativePath;
    const isImage = (obs.media && obs.media.mime || "").startsWith("image/");
    let preview;
    if (isImage) {
      preview = el("img", {
        class: "thumb",
        src: `/api/review/media/${encodeURIComponent(entry.id)}`,
        alt: `${label} (${obs.media.role || "unknown role"})`,
      });
      preview.addEventListener("error", () => {
        const fallback = el("span", { class: "thumb-broken", text: "Preview unavailable" });
        preview.replaceWith(fallback);
      });
    } else {
      preview = el("span", { class: "thumb-broken", text: "No preview" });
    }
    list.appendChild(el("li", {}, [preview, el("span", { text: label })]));
  }
}

export async function init(root, { api, announceStatus, announceError }) {
  const freshnessEl = root.querySelector("#overview-freshness");
  const rootsBody = root.querySelector("#overview-roots-body");
  const queuesList = root.querySelector("#overview-queues");
  const duplicatesDl = root.querySelector("#overview-duplicates");
  const issuesBody = root.querySelector("#overview-issues-body");
  const adaptersBody = root.querySelector("#overview-adapters-body");
  const artworkStrip = root.querySelector("#overview-artwork-strip");
  const refreshButton = root.querySelector("#overview-refresh");

  async function load() {
    const overview = await api.get("/api/review/overview");
    lastOverview = overview;
    renderFreshness(freshnessEl, overview);
    renderRoots(rootsBody, overview);
    renderQueues(queuesList, overview);
    renderDuplicateSummary(duplicatesDl, overview);
    renderIssues(issuesBody, overview);
    try {
      const adapters = await api.get("/api/review/adapters");
      renderAdapters(adaptersBody, adapters);
    } catch (err) {
      renderAdapters(adaptersBody, []);
    }
    try {
      const page = await api.get("/api/review/observations?pageSize=8");
      renderArtworkStrip(artworkStrip, page);
    } catch (err) {
      renderArtworkStrip(artworkStrip, null);
    }
  }

  try {
    await load();
  } catch (err) {
    if (err.code === "config_missing") {
      freshnessEl.textContent = "No active configuration saved yet. Complete first-run setup above, then return here.";
    } else {
      announceError(`Overview: failed to load the review snapshot (${err.message}).`);
    }
  }

  refreshButton.addEventListener("click", async () => {
    try {
      const result = await api.post("/api/review/refresh", {});
      if (result.status === "in-progress") {
        announceStatus("A refresh is already in progress.");
      } else {
        announceStatus(`Snapshot refreshed: ${result.observationCount || 0} observation(s) across ${result.rootCount || 0} root(s).`);
      }
      await load();
    } catch (err) {
      announceError(`Refresh failed: ${err.message}`);
    }
  });
}
