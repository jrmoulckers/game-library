// adapters.js: plan-only adapter readiness. This module only renders
// GET /api/review/adapters; it never adds a publish/apply control.

import { el, clear, yesNo } from "./dom.js";

export async function init(root, { api, announceError }) {
  const tbody = root.querySelector("#adapters-body");
  try {
    const adapters = await api.get("/api/review/adapters");
    clear(tbody);
    if (!adapters || adapters.length === 0) {
      tbody.appendChild(el("tr", {}, [el("td", { colspan: "4", text: "No adapter status available." })]));
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
  } catch (err) {
    announceError(`Adapters: failed to load status (${err.message}).`);
  }
}
