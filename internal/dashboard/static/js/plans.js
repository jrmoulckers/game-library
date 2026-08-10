// plans.js: generate import/bundle/export plans, run the exact manifest
// analysis Gate C needs, and record Gate B/C reviews. There is
// deliberately no "apply" affordance anywhere in this module — it does not
// exist as an endpoint on this server.

import { el, clear, formatBytes, yesNo } from "./dom.js";

let currentPlan = null;
let currentDigest = "";
let lastAnalysis = null;

function renderActions(tbody, actions) {
  clear(tbody);
  if (!actions || actions.length === 0) {
    tbody.appendChild(el("tr", {}, [el("td", { colspan: "6", text: "No actions in this plan." })]));
    return;
  }
  for (const a of actions) {
    const source = a.sourceRoot ? `${a.sourceRoot}: ${a.sourcePath || ""}` : "\u2014";
    const destination = a.destinationRoot ? `${a.destinationRoot}: ${a.destinationPath || ""}` : "\u2014";
    tbody.appendChild(
      el("tr", {}, [
        el("td", { text: a.action }),
        el("td", { text: source }),
        el("td", { text: destination }),
        el("td", { text: a.reason || "\u2014" }),
        el("td", { text: a.sourceSha256 || "\u2014" }),
        el("td", { text: a.sourceSize ? formatBytes(a.sourceSize) : "\u2014" }),
      ]),
    );
  }
}

function renderAnalysis(actionsBody, spaceBody, summary, analysis) {
  clear(actionsBody);
  summary.textContent = `${analysis.conflicts || 0} conflict(s) across ${(analysis.actions || []).length} action(s). Estimated new bytes: ${formatBytes(analysis.estimatedNewBytes || 0)}. Estimated backup bytes: ${formatBytes(analysis.estimatedBackupBytes || 0)}.`;
  for (const entry of analysis.actions || []) {
    const details = [];
    if (entry.currentDestinationHash) details.push(`current hash ${entry.currentDestinationHash}`);
    if (entry.conflictReason) details.push(entry.conflictReason);
    actionsBody.appendChild(
      el("tr", {}, [
        el("td", { text: entry.action.action }),
        el("td", { text: entry.effect }),
        el("td", { text: yesNo(entry.conflict), class: entry.conflict ? "state-warn" : "state-ok" }),
        el("td", { text: yesNo(entry.currentDestinationExists) }),
        el("td", { text: details.join("; ") || "\u2014" }),
      ]),
    );
  }
  clear(spaceBody);
  for (const d of analysis.destinations || []) {
    spaceBody.appendChild(
      el("tr", {}, [
        el("td", { text: d.root }),
        el("td", { text: formatBytes(d.neededBytes) }),
        el("td", { text: d.availableBytesKnown ? formatBytes(d.availableBytes) : "unknown" }),
        el("td", {
          text: d.availableBytesKnown ? yesNo(d.sufficient) : "unknown",
          class: d.availableBytesKnown ? (d.sufficient ? "state-ok" : "state-danger") : "",
        }),
      ]),
    );
  }
}

export async function init(root, { api, announceStatus, announceError }) {
  const generateStatus = root.querySelector("#plan-generate-status");
  const importButton = root.querySelector("#plan-generate-import");
  const bundleProfileInput = root.querySelector("#plan-bundle-profile");
  const bundleButton = root.querySelector("#plan-generate-bundle");
  const exportProfileInput = root.querySelector("#plan-export-profile");
  const exportAdapterSelect = root.querySelector("#plan-export-adapter");
  const exportButton = root.querySelector("#plan-generate-export");

  const currentSummary = root.querySelector("#plan-current-summary");
  const actionsBody = root.querySelector("#plan-actions-body");
  const analyzeButton = root.querySelector("#plan-analyze-button");
  const analysisSummary = root.querySelector("#plan-analysis-summary");
  const analysisBody = root.querySelector("#plan-analysis-body");
  const spaceBody = root.querySelector("#plan-space-body");

  const gateBForm = root.querySelector("#gate-b-form");
  const gateBPolicyDigest = root.querySelector("#gate-b-policy-digest");
  const gateBStatus = root.querySelector("#gate-b-status");
  const gateCForm = root.querySelector("#gate-c-form");
  const gateCSubmit = root.querySelector("#gate-c-submit");
  const gateCStatus = root.querySelector("#gate-c-status");

  try {
    const cfgView = await api.get("/api/config");
    if (cfgView.policyDigest) gateBPolicyDigest.value = cfgView.policyDigest;
  } catch (err) {
    // Non-fatal: the reviewer can paste a policy digest manually.
  }

  function setPlan(view) {
    currentPlan = view.plan;
    currentDigest = view.digest || "";
    currentSummary.textContent = `Plan kind "${currentPlan.kind}" with ${(currentPlan.actions || []).length} action(s). Digest: ${currentDigest}. Artifact: ${view.artifact}.`;
    renderActions(actionsBody, currentPlan.actions);
    analyzeButton.disabled = false;
    lastAnalysis = null;
    gateCSubmit.disabled = true;
    gateCSubmit.textContent = "Analyze a plan above to create a Gate C review";
    analysisSummary.textContent = "";
    clear(analysisBody);
    clear(spaceBody);
  }

  importButton.addEventListener("click", async () => {
    try {
      const view = await api.post("/api/review/plans/import", {});
      setPlan(view);
      generateStatus.textContent = `Import plan ${view.created ? "created" : "already existed with identical content"}.`;
      announceStatus("Import plan generated.");
    } catch (err) {
      generateStatus.textContent = `Failed to generate an import plan: ${err.message}`;
      announceError(`Import plan generation failed: ${err.message}`);
    }
  });

  bundleButton.addEventListener("click", async () => {
    const profileId = bundleProfileInput.value.trim();
    if (!profileId) {
      generateStatus.textContent = "Enter a profile id to generate a bundle plan.";
      return;
    }
    try {
      const view = await api.post("/api/review/plans/bundle", { profileId });
      setPlan(view);
      generateStatus.textContent = `Bundle plan ${view.created ? "created" : "already existed with identical content"} for profile ${profileId}.`;
      announceStatus("Bundle plan generated.");
    } catch (err) {
      generateStatus.textContent = `Failed to generate a bundle plan: ${err.message}`;
      announceError(`Bundle plan generation failed: ${err.message}`);
    }
  });

  exportButton.addEventListener("click", async () => {
    const profileId = exportProfileInput.value.trim();
    if (!profileId) {
      generateStatus.textContent = "Enter a profile id to generate an export plan.";
      return;
    }
    try {
      const view = await api.post("/api/review/plans/export", { profileId, adapter: exportAdapterSelect.value });
      setPlan(view);
      generateStatus.textContent = `Export plan ${view.created ? "created" : "already existed with identical content"} for profile ${profileId} (${exportAdapterSelect.value}).`;
      announceStatus("Export plan generated.");
    } catch (err) {
      generateStatus.textContent = `Failed to generate an export plan: ${err.message}`;
      announceError(`Export plan generation failed: ${err.message}`);
    }
  });

  analyzeButton.addEventListener("click", async () => {
    if (!currentPlan) return;
    try {
      const analysis = await api.post("/api/review/manifest-analysis", { manifest: currentPlan });
      lastAnalysis = analysis;
      renderAnalysis(analysisBody, spaceBody, analysisSummary, analysis);
      gateCSubmit.disabled = false;
      gateCSubmit.textContent = "Create Gate C review";
      announceStatus("Manifest analysis complete.");
    } catch (err) {
      analysisSummary.textContent = `Analysis failed: ${err.message}`;
      announceError(`Manifest analysis failed: ${err.message}`);
    }
  });

  gateBForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const gateAId = gateBForm.querySelector("#gate-b-gate-a-id").value.trim();
    const policyDigest = gateBPolicyDigest.value.trim();
    const profileDigest = gateBForm.querySelector("#gate-b-profile-digest").value.trim();
    const exportPlanDigest = gateBForm.querySelector("#gate-b-export-digest").value.trim();
    const reviewer = gateBForm.querySelector("#gate-b-reviewer").value;
    const notes = gateBForm.querySelector("#gate-b-notes").value;
    try {
      const result = await api.post("/api/review/gates/b", {
        gateAId,
        policyDigest,
        profileDigest: profileDigest || undefined,
        exportPlanDigest: exportPlanDigest || undefined,
        reviewer,
        notes,
      });
      gateCForm.querySelector("#gate-c-gate-b-id").value = result.review.id;
      gateBStatus.textContent = `Gate B review ${result.review.id} recorded (${result.created ? "new" : "already existed"}). Artifact: ${result.artifact}.`;
      announceStatus("Gate B review recorded.");
    } catch (err) {
      gateBStatus.textContent = `Failed to create the Gate B review: ${err.message}`;
      announceError(`Gate B review failed: ${err.message}`);
    }
  });

  gateCForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (!lastAnalysis) {
      gateCStatus.textContent = "Analyze the current plan before creating a Gate C review.";
      return;
    }
    const gateBId = gateCForm.querySelector("#gate-c-gate-b-id").value.trim();
    const rollbackPlan = gateCForm.querySelector("#gate-c-rollback-plan").value;
    const reviewer = gateCForm.querySelector("#gate-c-reviewer").value;
    const notes = gateCForm.querySelector("#gate-c-notes").value;
    try {
      const result = await api.post("/api/review/gates/c", {
        gateBId,
        analysis: lastAnalysis,
        rollbackPlan,
        reviewer,
        notes,
      });
      gateCStatus.textContent = `Gate C review ${result.review.id} recorded with executable=${result.review.executable} (always false). Artifact: ${result.artifact}. There is no apply step after this.`;
      announceStatus("Gate C review recorded. No apply step follows.");
    } catch (err) {
      gateCStatus.textContent = `Failed to create the Gate C review: ${err.message}`;
      announceError(`Gate C review failed: ${err.message}`);
    }
  });
}
