// policy.js: active policy summary, the local policy draft rule editor
// (source/system/role/assetSha256/mode with explicit precedence weights),
// save-with-conflict-recovery, and an impact preview against either the
// draft being edited or the active policy.

import { el, clear } from "./dom.js";

let draftBaseDigest = "";
let schemaVersion = 1;
let rules = [];
let ruleSeq = 0;

function newRule(rule) {
  ruleSeq += 1;
  return {
    key: ruleSeq,
    source: rule?.source || "",
    system: rule?.system || "",
    role: rule?.role || "",
    assetSha256: rule?.assetSha256 || "",
    mode: rule?.mode || "managed",
  };
}

const MODES = ["managed", "tracked-external", "promote-on-approval", "quarantined"];

function modeSelect(rule, label) {
  return el(
    "select",
    {
      "aria-label": label,
      onchange: (e) => {
        rule.mode = e.target.value;
      },
    },
    MODES.map((m) => el("option", { value: m, text: m, selected: m === rule.mode ? "selected" : undefined })),
  );
}

function renderRules(tbody) {
  clear(tbody);
  if (rules.length === 0) {
    tbody.appendChild(el("tr", {}, [el("td", { colspan: "6", text: "No rules yet. The global default applies to everything." })]));
    return;
  }
  rules.forEach((rule, index) => {
    const field = (name, value, setter) =>
      el("input", {
        type: "text",
        "aria-label": `Rule ${index + 1} ${name}`,
        value,
        oninput: (e) => setter(e.target.value),
      });
    const removeButton = el("button", {
      type: "button",
      text: "Remove",
      "aria-label": `Remove rule ${index + 1}`,
      onclick: () => {
        rules = rules.filter((r) => r.key !== rule.key);
        renderRules(tbody);
        const nextRow = tbody.querySelectorAll("tr")[Math.min(index, rules.length - 1)];
        const nextTarget = nextRow?.querySelector("input, select, button") || document.getElementById("policy-add-rule");
        nextTarget?.focus();
      },
    });
    tbody.appendChild(
      el("tr", {}, [
        el("td", {}, [field("source", rule.source, (v) => (rule.source = v))]),
        el("td", {}, [field("system", rule.system, (v) => (rule.system = v))]),
        el("td", {}, [field("role", rule.role, (v) => (rule.role = v))]),
        el("td", {}, [field("asset SHA-256", rule.assetSha256, (v) => (rule.assetSha256 = v))]),
        el("td", {}, [modeSelect(rule, `Rule ${index + 1} mode`)]),
        el("td", {}, [removeButton]),
      ]),
    );
  });
}

function currentPolicyFile(defaultMode) {
  return {
    version: schemaVersion,
    default: defaultMode,
    rules: rules.map((r) => ({
      source: r.source || undefined,
      system: r.system || undefined,
      role: r.role || undefined,
      assetSha256: r.assetSha256 || undefined,
      mode: r.mode,
    })),
  };
}

function renderImpact(tbody, summaryEl, impact) {
  clear(tbody);
  const counts = impact.counts || {};
  const parts = Object.entries(counts)
    .map(([mode, count]) => `${mode}: ${count}`)
    .join(", ");
  summaryEl.textContent = parts ? `Winning-mode counts \u2014 ${parts}` : "No observations to evaluate yet.";
  const entries = impact.entries || [];
  if (entries.length === 0) {
    tbody.appendChild(el("tr", {}, [el("td", { colspan: "3", text: "No observations to evaluate yet." })]));
    return;
  }
  for (const entry of entries.slice(0, 200)) {
    const winner = entry.matchedRule ? `Rule ${entry.matchedRuleIndex}` : "Global default";
    tbody.appendChild(
      el("tr", {}, [
        el("td", { text: `${entry.rootId}: ${entry.relativePath}` }),
        el("td", { text: entry.mode }),
        el("td", { text: winner }),
      ]),
    );
  }
  if (entries.length > 200) {
    tbody.appendChild(
      el("tr", {}, [el("td", { colspan: "3", text: `\u2026and ${entries.length - 200} more (see the artwork browser for full filtering).` })]),
    );
  }
}

export async function init(root, { api, announceStatus, announceError }) {
  const activeSummary = root.querySelector("#policy-active-summary");
  const form = root.querySelector("#policy-form");
  const defaultModeSelect = root.querySelector("#policy-default-mode");
  const rulesBody = root.querySelector("#policy-rules-body");
  const addRuleButton = root.querySelector("#policy-add-rule");
  const previewButton = root.querySelector("#policy-preview");
  const statusEl = root.querySelector("#policy-status");
  const impactSummary = root.querySelector("#policy-impact-summary");
  const impactBody = root.querySelector("#policy-impact-body");

  try {
    const cfgView = await api.get("/api/config");
    clear(activeSummary);
    if (cfgView.exists && cfgView.config) {
      const p = cfgView.config.policy;
      activeSummary.appendChild(el("dt", { text: "Default mode" }));
      activeSummary.appendChild(el("dd", { text: p.default }));
      activeSummary.appendChild(el("dt", { text: "Rule count" }));
      activeSummary.appendChild(el("dd", { text: String((p.rules || []).length) }));
      activeSummary.appendChild(el("dt", { text: "Policy digest" }));
      activeSummary.appendChild(el("dd", { text: cfgView.policyDigest || "unknown" }));
    } else {
      activeSummary.appendChild(el("dt", { text: "Active policy" }));
      activeSummary.appendChild(el("dd", { text: "No active configuration saved yet." }));
    }
  } catch (err) {
    announceError(`Policy: failed to load the active configuration (${err.message}).`);
  }

  try {
    const draftView = await api.get("/api/drafts/policy");
    if (draftView.exists && draftView.draft) {
      draftBaseDigest = draftView.draft.digest || "";
      schemaVersion = draftView.draft.policy.version || schemaVersion;
      defaultModeSelect.value = draftView.draft.policy.default || "tracked-external";
      rules = (draftView.draft.policy.rules || []).map(newRule);
    }
  } catch (err) {
    announceError(`Policy: failed to load the policy draft (${err.message}).`);
  }
  renderRules(rulesBody);

  addRuleButton.addEventListener("click", () => {
    rules.push(newRule());
    renderRules(rulesBody);
  });

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const policy = currentPolicyFile(defaultModeSelect.value);
    try {
      const envelope = await api.put("/api/drafts/policy", { baseDigest: draftBaseDigest, policy });
      draftBaseDigest = envelope.digest || "";
      statusEl.textContent = `Policy draft saved (digest ${envelope.digest}). This is a local draft only; nothing was promoted.`;
      announceStatus("Policy draft saved.");
    } catch (err) {
      if (err.code === "draft_conflict") {
        statusEl.textContent = "Save conflict: the policy draft changed elsewhere. Reloading the current draft \u2014 review it and save again.";
        announceError("Policy draft save conflict: reloaded the current draft.");
        try {
          const draftView = await api.get("/api/drafts/policy");
          if (draftView.exists && draftView.draft) {
            draftBaseDigest = draftView.draft.digest || "";
            defaultModeSelect.value = draftView.draft.policy.default || "tracked-external";
            rules = (draftView.draft.policy.rules || []).map(newRule);
            renderRules(rulesBody);
          }
        } catch (reloadErr) {
          announceError(`Failed to reload the policy draft after a conflict: ${reloadErr.message}`);
        }
      } else {
        statusEl.textContent = `Save failed: ${err.message}`;
        announceError(`Failed to save the policy draft: ${err.message}`);
      }
    }
  });

  previewButton.addEventListener("click", async () => {
    const policy = currentPolicyFile(defaultModeSelect.value);
    try {
      const impact = await api.post("/api/review/policy-impact-preview", { policy });
      renderImpact(impactBody, impactSummary, impact);
      announceStatus("Policy impact preview updated.");
    } catch (err) {
      impactSummary.textContent = `Preview failed: ${err.message}`;
      announceError(`Policy impact preview failed: ${err.message}`);
    }
  });
}
