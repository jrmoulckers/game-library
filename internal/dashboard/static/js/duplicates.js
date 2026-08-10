// duplicates.js: exact-hash duplicate groups with a server-computed
// classification and explanation. There is no delete control here, by
// design (ADR-0007): this module only renders review.BuildDuplicateView.

import { el, clear, formatBytes, emptyRow } from "./dom.js";

const CLASS_LABELS = {
  "canonical-opportunity": "Canonical opportunity",
  "expected-generated-or-export-copy": "Expected generated/export copy",
  review: "Needs review",
};

function classDescription(classification) {
  return CLASS_LABELS[classification] || classification;
}

export async function init(root, { api, announceError }) {
  const summary = root.querySelector("#duplicates-summary");
  const tbody = root.querySelector("#duplicates-body");

  try {
    const groups = await api.get("/api/review/duplicates");
    summary.textContent = `${groups.length} duplicate group(s) found.`;
    clear(tbody);
    if (groups.length === 0) {
      tbody.appendChild(emptyRow(5, "No exact-hash duplicate groups found."));
      return;
    }
    for (const group of groups) {
      const occurrences = el(
        "ul",
        { class: "occurrence-list" },
        (group.occurrences || []).map((occ) =>
          el("li", { text: `${occ.rootId} (${occ.rootKind}): ${occ.relativePath}` }),
        ),
      );
      tbody.appendChild(
        el("tr", {}, [
          el("td", { text: group.sha256 }),
          el("td", { text: formatBytes(group.size) }),
          el("td", { text: classDescription(group.classification) }),
          el("td", { text: group.reason }),
          el("td", {}, [occurrences]),
        ]),
      );
    }
  } catch (err) {
    if (err.code === "config_missing") {
      summary.textContent = "No active configuration saved yet.";
    } else {
      announceError(`Duplicates: failed to load groups (${err.message}).`);
    }
  }
}
