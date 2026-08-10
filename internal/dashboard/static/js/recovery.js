// recovery.js: immutable local plan/gate-review history with digest and
// integrity status. There is no applied/rolled-back history and no
// prune/delete/apply control anywhere in this module (ADR-0007): it only
// renders review.ListHistory.

import { el, clear, yesNo } from "./dom.js";

export async function init(root, { api, announceError }) {
  const tbody = root.querySelector("#recovery-body");
  try {
    const entries = await api.get("/api/review/history");
    clear(tbody);
    if (!entries || entries.length === 0) {
      tbody.appendChild(el("tr", {}, [el("td", { colspan: "6", text: "No local plan or gate-review artifacts recorded yet." })]));
      return;
    }
    for (const entry of entries) {
      tbody.appendChild(
        el("tr", {}, [
          el("td", { text: entry.type }),
          el("td", { text: entry.id }),
          el("td", { text: entry.kind || "\u2014" }),
          el("td", { text: entry.digest }),
          el("td", { text: entry.createdAt || "\u2014" }),
          el("td", { text: yesNo(entry.verified), class: entry.verified ? "state-ok" : "state-danger" }),
        ]),
      );
    }
  } catch (err) {
    announceError(`Recovery: failed to load local artifact history (${err.message}).`);
  }
}
