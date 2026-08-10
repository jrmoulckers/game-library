// identity.js: the identity review queue. Deterministic ("high"
// confidence) proposals are shown separately from "proposal"-confidence
// items and unmapped items, which always land in the "needs review" table
// — this module never merges or upgrades a proposal's confidence itself.
// "Reviewed" checkboxes are purely client-side state; they gate the Gate A
// submit button but are never sent to the server, which only ever records
// the two content digests.

import { el, clear, emptyRow } from "./dom.js";

const reviewed = new Set();

function evidenceText(proposal) {
  return `${proposal.mappingType || "unknown mapping"}: ${proposal.reason}`;
}

function renderReviewCheckbox(id, onChange) {
  return el("input", {
    type: "checkbox",
    "aria-label": `Mark ${id} reviewed`,
    checked: reviewed.has(id),
    onchange: (e) => {
      if (e.target.checked) reviewed.add(id);
      else reviewed.delete(id);
      onChange();
    },
  });
}

export async function init(root, { api, announceStatus, announceError }) {
  const deterministicBody = root.querySelector("#identity-deterministic-body");
  const reviewBody = root.querySelector("#identity-review-body");
  const selectionSummary = root.querySelector("#gate-a-selection-summary");
  const gateForm = root.querySelector("#gate-a-form");
  const submitButton = root.querySelector("#gate-a-submit");
  const gateStatus = root.querySelector("#gate-a-status");

  let allIDs = [];
  let inventoryDigest = "";
  let identityDigest = "";

  function updateSelectionSummary() {
    const total = allIDs.length;
    const done = allIDs.filter((id) => reviewed.has(id)).length;
    selectionSummary.textContent = `${done} of ${total} listed item(s) marked reviewed.`;
    submitButton.disabled = !(total > 0 && done === total);
    submitButton.textContent =
      total > 0 && done === total
        ? "Create Gate A review"
        : "Mark every listed item reviewed to create a Gate A review";
  }

  try {
    const view = await api.get("/api/review/identity");
    inventoryDigest = view.inventoryDigest || "";
    identityDigest = view.identityDigest || "";
    reviewed.clear();
    allIDs = [];

    clear(deterministicBody);
    const deterministic = (view.proposals || []).filter((p) => p.proposal.confidence === "high");
    if (deterministic.length === 0) {
      deterministicBody.appendChild(emptyRow(5, "No deterministic proposals."));
    } else {
      for (const entry of deterministic) {
        allIDs.push(entry.id);
        deterministicBody.appendChild(
          el("tr", {}, [
            el("td", {}, [renderReviewCheckbox(entry.id, updateSelectionSummary)]),
            el("td", { text: entry.proposal.relativePath }),
            el("td", { text: entry.proposal.canonicalId || "\u2014" }),
            el("td", { text: entry.proposal.confidence }),
            el("td", { text: evidenceText(entry.proposal) }),
          ]),
        );
      }
    }

    clear(reviewBody);
    const needsReview = (view.proposals || []).filter((p) => p.proposal.confidence !== "high");
    const unmapped = view.unmapped || [];
    if (needsReview.length === 0 && unmapped.length === 0) {
      reviewBody.appendChild(emptyRow(5, "Nothing needs review right now."));
    } else {
      for (const entry of needsReview) {
        allIDs.push(entry.id);
        reviewBody.appendChild(
          el("tr", {}, [
            el("td", {}, [renderReviewCheckbox(entry.id, updateSelectionSummary)]),
            el("td", { text: entry.proposal.relativePath }),
            el("td", { text: entry.proposal.canonicalId || "(none proposed)" }),
            el("td", { text: entry.proposal.confidence || "low" }),
            el("td", { text: evidenceText(entry.proposal) }),
          ]),
        );
      }
      for (const entry of unmapped) {
        allIDs.push(entry.id);
        reviewBody.appendChild(
          el("tr", {}, [
            el("td", {}, [renderReviewCheckbox(entry.id, updateSelectionSummary)]),
            el("td", { text: entry.item.relativePath }),
            el("td", { text: "(no proposal)" }),
            el("td", { text: "unmapped" }),
            el("td", { text: entry.item.reason }),
          ]),
        );
      }
    }

    updateSelectionSummary();
  } catch (err) {
    if (err.code === "config_missing") {
      clear(deterministicBody);
      deterministicBody.appendChild(emptyRow(5, "No active configuration saved yet."));
      clear(reviewBody);
      reviewBody.appendChild(emptyRow(5, "No active configuration saved yet."));
    } else {
      announceError(`Identity queue: failed to load proposals (${err.message}).`);
    }
  }

  gateForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (!inventoryDigest || !identityDigest) {
      gateStatus.textContent = "Cannot create a Gate A review: inventory/identity digests are unavailable.";
      return;
    }
    const reviewer = gateForm.querySelector("#gate-a-reviewer").value;
    const notes = gateForm.querySelector("#gate-a-notes").value;
    try {
      const result = await api.post("/api/review/gates/a", {
        inventoryDigest,
        identityDigest,
        reviewer,
        notes,
      });
      gateStatus.textContent = `Gate A review ${result.review.id} recorded (${result.created ? "new" : "already existed with identical content"}). Artifact: ${result.artifact}.`;
      announceStatus("Gate A review recorded.");
    } catch (err) {
      gateStatus.textContent = `Failed to create the Gate A review: ${err.message}`;
      announceError(`Gate A review failed: ${err.message}`);
    }
  });
}
