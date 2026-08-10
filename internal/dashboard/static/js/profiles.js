// profiles.js: list/create/edit profile drafts, a closure (resolve)
// preview, and an adapter export preview. Games/mods are edited as
// JSON-friendly structured text (validated client-side as JSON before
// sending) rather than a fully bespoke widget per nested field, since the
// canonical shape is already the profile JSON schema this repository
// already validates server-side (profile.Validate) — this module never
// re-implements that validation, only catches JSON.parse syntax errors
// early so a save round-trip isn't required to see them.

import { el, clear } from "./dom.js";

let currentProfile = null; // the full model.Profile object currently loaded
let baseDigest = "";
let schemaVersion = 1;

function renderList(tbody, profiles, onOpen) {
  clear(tbody);
  if (!profiles || profiles.length === 0) {
    tbody.appendChild(el("tr", {}, [el("td", { colspan: "4", text: "No profile drafts saved yet." })]));
    return;
  }
  for (const p of profiles) {
    tbody.appendChild(
      el("tr", {}, [
        el("td", { text: p.id }),
        el("td", { text: p.name }),
        el("td", { text: p.theme || "\u2014" }),
        el("td", {}, [el("button", { type: "button", text: "Open", onclick: () => onOpen(p.id) })]),
      ]),
    );
  }
}

export async function init(root, { api, announceStatus, announceError }) {
  const listBody = root.querySelector("#profiles-list-body");
  const selectForm = root.querySelector("#profiles-select-form");
  const idInput = root.querySelector("#profiles-id-input");

  const editorForm = root.querySelector("#profile-editor-form");
  const editorIdLabel = root.querySelector("#profile-editor-id");
  const nameInput = root.querySelector("#profile-name");
  const descriptionInput = root.querySelector("#profile-description");
  const themeSelect = root.querySelector("#profile-theme");
  const gamesJsonInput = root.querySelector("#profile-games-json");
  const gamesError = root.querySelector("#profile-games-error");
  const modsJsonInput = root.querySelector("#profile-mods-json");
  const modsError = root.querySelector("#profile-mods-error");
  const editorStatus = root.querySelector("#profile-editor-status");

  const resolveButton = root.querySelector("#profile-resolve-button");
  const resolveResult = root.querySelector("#profile-resolve-result");
  const exportForm = root.querySelector("#profile-export-form");
  const exportResult = root.querySelector("#profile-export-result");

  async function refreshList() {
    try {
      const profiles = await api.get("/api/review/profiles");
      renderList(listBody, profiles, loadProfile);
    } catch (err) {
      announceError(`Profiles: failed to list saved drafts (${err.message}).`);
    }
  }

  try {
    const themesView = await api.get("/api/review/themes");
    for (const theme of themesView.themes || []) {
      themeSelect.appendChild(el("option", { value: theme, text: theme }));
    }
  } catch (err) {
    // A missing/unconfigured catalog root is not fatal here: the theme
    // list is simply empty (the free-text default option remains).
  }

  await refreshList();

  function openEditor(id, profile, digest) {
    currentProfile = profile;
    baseDigest = digest;
    editorForm.hidden = false;
    exportForm.hidden = false;
    editorIdLabel.textContent = id;
    nameInput.value = profile.name || "";
    descriptionInput.value = profile.description || "";
    themeSelect.value = profile.theme || "";
    gamesJsonInput.value = JSON.stringify(profile.games || [], null, 2);
    modsJsonInput.value = JSON.stringify(profile.mods || [], null, 2);
    gamesError.textContent = "";
    modsError.textContent = "";
    editorStatus.textContent = "";
    resolveButton.disabled = false;
    resolveResult.textContent = "";
    exportResult.textContent = "";
  }

  async function loadProfile(id) {
    try {
      const view = await api.get(`/api/drafts/profiles/${encodeURIComponent(id)}`);
      if (view.exists && view.draft) {
        schemaVersion = view.draft.profile.version || schemaVersion;
        openEditor(id, view.draft.profile, view.draft.digest || "");
        announceStatus(`Loaded profile draft ${id}.`);
      } else {
        openEditor(id, { version: schemaVersion, id, name: "", games: [] }, "");
        announceStatus(`Starting a new profile draft ${id}.`);
      }
    } catch (err) {
      announceError(`Profiles: failed to load draft ${id} (${err.message}).`);
    }
  }

  selectForm.addEventListener("submit", (event) => {
    event.preventDefault();
    const id = idInput.value.trim();
    if (id) loadProfile(id);
  });

  editorForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (!currentProfile) return;
    let games;
    let mods;
    gamesError.textContent = "";
    modsError.textContent = "";
    try {
      games = JSON.parse(gamesJsonInput.value || "[]");
    } catch (err) {
      gamesError.textContent = `Games JSON is not valid: ${err.message}`;
      return;
    }
    try {
      mods = JSON.parse(modsJsonInput.value || "[]");
    } catch (err) {
      modsError.textContent = `Mods JSON is not valid: ${err.message}`;
      return;
    }
    const profile = {
      version: schemaVersion,
      id: currentProfile.id,
      name: nameInput.value,
      description: descriptionInput.value || undefined,
      theme: themeSelect.value || undefined,
      games,
      mods: mods.length ? mods : undefined,
      compatibility: currentProfile.compatibility,
    };
    try {
      const envelope = await api.put(`/api/drafts/profiles/${encodeURIComponent(currentProfile.id)}`, {
        baseDigest,
        profile,
      });
      baseDigest = envelope.digest || "";
      currentProfile = envelope.profile;
      editorStatus.textContent = `Profile draft saved (digest ${envelope.digest}). This is a local draft only; nothing was published.`;
      announceStatus("Profile draft saved.");
      await refreshList();
    } catch (err) {
      if (err.code === "draft_conflict") {
        editorStatus.textContent = "Save conflict: this draft changed elsewhere. Reloading the current draft \u2014 review it and save again.";
        announceError("Profile draft save conflict: reloaded the current draft.");
        await loadProfile(currentProfile.id);
      } else {
        editorStatus.textContent = `Save failed: ${err.message}`;
        announceError(`Failed to save the profile draft: ${err.message}`);
      }
    }
  });

  resolveButton.addEventListener("click", async () => {
    if (!currentProfile) return;
    try {
      const resolution = await api.get(`/api/review/profiles/${encodeURIComponent(currentProfile.id)}/resolve`);
      clear(resolveResult);
      const summary = el("p", {
        text: `Complete: ${resolution.complete ? "yes" : "no"}. Revision: ${resolution.revision}. ${(resolution.assets || []).length} asset(s).`,
      });
      const table = el("table", {}, [
        el("caption", { text: `Closure preview for profile ${currentProfile.id}` }),
        el("thead", {}, [
          el("tr", {}, [
            el("th", { scope: "col", text: "Game" }),
            el("th", { scope: "col", text: "Role" }),
            el("th", { scope: "col", text: "Available" }),
          ]),
        ]),
        el(
          "tbody",
          {},
          (resolution.assets || []).map((a) =>
            el("tr", {}, [
              el("td", { text: a.gameId }),
              el("td", { text: a.role }),
              el("td", { text: a.available ? "Yes" : "No" }),
            ]),
          ),
        ),
      ]);
      resolveResult.appendChild(summary);
      resolveResult.appendChild(table);
      if ((resolution.issues || []).length) {
        resolveResult.appendChild(
          el(
            "ul",
            {},
            resolution.issues.map((issue) => el("li", { text: `${issue.code}: ${issue.message}` })),
          ),
        );
      }
    } catch (err) {
      resolveResult.textContent = `Resolve failed: ${err.message}`;
      announceError(`Profile resolve failed: ${err.message}`);
    }
  });

  exportForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (!currentProfile) return;
    const adapter = exportForm.querySelector("#profile-export-adapter").value;
    try {
      const preview = await api.get(`/api/review/profiles/${encodeURIComponent(currentProfile.id)}/export/${encodeURIComponent(adapter)}`);
      clear(exportResult);
      const plan = preview.Plan || {};
      exportResult.appendChild(
        el("p", { text: `Plan-only export preview for ${adapter}: ${(plan.actions || []).length} action(s), kind "${plan.kind || "unknown"}".` }),
      );
      if (preview.DeckyProfile) {
        const d = preview.DeckyProfile;
        exportResult.appendChild(
          el("p", {
            text: `Decky v1 preview \u2014 id: ${d.id}, artwork: ${d.artwork === null ? "null" : d.artwork}, mods: ${d.mods ? d.mods.length : 0}, grid artwork present: ${preview.HasGridArtwork ? "yes" : "no (uses .deck-profile-empty)"}.`,
          }),
        );
      }
      exportResult.appendChild(
        el("p", { text: "This preview never writes to a live frontend; it is a plan-only export description." }),
      );
    } catch (err) {
      exportResult.textContent = `Export preview failed: ${err.message}`;
      announceError(`Profile export preview failed: ${err.message}`);
    }
  });
}
