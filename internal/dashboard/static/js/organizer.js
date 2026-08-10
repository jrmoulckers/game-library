import { clear, el, formatBytes } from "./dom.js";

let catalog = null;
let activePlatform = "";
let activeGame = null;
let scanPolling = false;
const dialogOpeners = new WeakMap();

function mediaURL(id) {
  return `/api/review/media/${encodeURIComponent(id)}`;
}

function image(id, alt, className = "") {
  const img = el("img", { src: mediaURL(id), alt, loading: "lazy", class: className });
  img.addEventListener("error", () => {
    img.hidden = true;
    const fallback = el("span", { class: "broken-art", text: "Preview unavailable" });
    img.parentElement?.appendChild(fallback);
  }, { once: true });
  return img;
}

function showView(id, moveFocus = true) {
  document.getElementById("advanced").open = false;
  for (const view of document.querySelectorAll(".organizer-view")) {
    view.hidden = view.id !== id;
  }
  for (const link of document.querySelectorAll(".stage-rail__list a")) {
    if (link.classList.contains("nav-primary")) {
      if (link.getAttribute("href") === `#${id}`) link.setAttribute("aria-current", "page");
      else link.removeAttribute("aria-current");
    }
  }
  document.getElementById(id)?.scrollIntoView({ block: "start" });
  const heading = document.querySelector(`#${CSS.escape(id)} h2`);
  if (heading && moveFocus) {
    heading.setAttribute("tabindex", "-1");
    heading.focus({ preventScroll: true });
  }
}

function platformCard(platform) {
  const mosaic = el("div", { class: "platform-mosaic", "aria-hidden": "true" });
  for (const id of platform.previewIds || []) {
    mosaic.appendChild(image(id, ""));
  }
  while (mosaic.childElementCount < 4) {
    mosaic.appendChild(el("span", { class: "mosaic-placeholder" }));
  }
  const button = el("button", {
    type: "button",
    class: "platform-card",
    onclick: () => openPlatform(platform.id),
    "aria-label": `Open ${platform.name}, ${platform.gameCount} games`,
  }, [
    mosaic,
    el("span", { class: "platform-card__body" }, [
      el("strong", { text: platform.name }),
      el("span", { text: `${platform.gameCount} ${platform.gameCount === 1 ? "game" : "games"}` }),
      el("span", {
        class: platform.missingArtCount ? "coverage coverage--missing" : "coverage",
        text: platform.missingArtCount
          ? `${platform.missingArtCount} missing artwork`
          : `${platform.coverage}% artwork coverage`,
      }),
    ]),
  ]);
  return button;
}

function renderLibrary() {
  const grid = document.getElementById("platform-grid");
  const summary = document.getElementById("library-summary");
  clear(grid);
  const gameCount = (catalog.games || []).length;
  summary.textContent = `${gameCount} ${gameCount === 1 ? "game" : "games"} across ${(catalog.platforms || []).length} platforms.`;
  document.getElementById("library-grid-status").textContent =
    `${(catalog.platforms || []).length} ${(catalog.platforms || []).length === 1 ? "platform" : "platforms"} loaded.`;
  if (catalog.needsAttention) {
    summary.append(` ${catalog.needsAttention} ${catalog.needsAttention === 1 ? "item needs" : "items need"} attention.`);
  }
  for (const platform of catalog.platforms || []) grid.appendChild(platformCard(platform));
}

function visibleGames() {
  const search = document.getElementById("game-search").value.trim().toLocaleLowerCase();
  const coverage = document.getElementById("game-coverage").value;
  const role = document.getElementById("game-role").value;
  const source = document.getElementById("game-source").value;
  const sort = document.getElementById("game-sort").value;
  const games = (catalog.games || []).filter((game) => {
    if (game.platformId !== activePlatform) return false;
    if (search && !game.title.toLocaleLowerCase().includes(search)) return false;
    if (coverage === "has-art" && game.assets.length === 0) return false;
    if (coverage === "missing" && game.missingRoles.length === 0) return false;
    if (role && !game.assets.some((asset) => asset.role === role)) return false;
    if (source && !game.assets.some((asset) => asset.sourceId === source)) return false;
    return true;
  });
  games.sort((a, b) => {
    if (sort === "coverage") return a.missingRoles.length - b.missingRoles.length || a.title.localeCompare(b.title);
    return a.title.localeCompare(b.title);
  });
  return games;
}

function representativeAsset(game) {
  return game.assets.find((asset) => ["portrait", "cover", "grid", "miximage"].includes(asset.role)) || game.assets[0];
}

function gameTile(game) {
  const asset = representativeAsset(game);
  return el("button", {
    type: "button", class: "game-tile", onclick: () => openGame(game),
    "aria-label": `Open ${game.title}`,
  }, [
    el("span", { class: "game-cover" }, asset
      ? [image(asset.id, `${game.title} ${asset.role}`)]
      : [el("span", { class: "cover-placeholder", text: "Missing cover" })]),
    el("strong", { text: game.title }),
    el("span", {
      class: "game-tile__meta",
      text: game.missingRoles.length ? `${game.assets.length} artwork ${game.assets.length === 1 ? "role" : "roles"}` : "Artwork complete",
    }),
  ]);
}

function renderGameGrid() {
  const grid = document.getElementById("game-grid");
  clear(grid);
  const games = visibleGames();
  if (!games.length) {
    grid.appendChild(el("p", { class: "empty-state", text: "No games match these filters." }));
    document.getElementById("game-grid-status").textContent = "No games match these filters.";
    return;
  }
  for (const game of games) grid.appendChild(gameTile(game));
  document.getElementById("game-grid-status").textContent =
    `${games.length} ${games.length === 1 ? "game" : "games"} shown.`;
}

function openPlatform(id) {
  activePlatform = id;
  const platform = catalog.platforms.find((item) => item.id === id);
  document.getElementById("platform-detail-heading").textContent = platform.name;
  document.getElementById("platform-summary").textContent =
    `${platform.gameCount} games · ${platform.artworkCount} artwork files · ${platform.coverage}% have artwork`;
  const games = (catalog.games || []).filter((game) => game.platformId === id);
  const roleSelect = document.getElementById("game-role");
  const sourceSelect = document.getElementById("game-source");
  clear(roleSelect);
  clear(sourceSelect);
  roleSelect.appendChild(el("option", { value: "", text: "Any role" }));
  sourceSelect.appendChild(el("option", { value: "", text: "Any source" }));
  const roles = new Set();
  const sources = new Map();
  for (const game of games) {
    for (const asset of game.assets) {
      roles.add(asset.role);
      sources.set(asset.sourceId, asset.sourceName);
    }
  }
  for (const role of [...roles].sort()) roleSelect.appendChild(el("option", { value: role, text: role }));
  for (const [sourceID, name] of [...sources].sort((a, b) => a[1].localeCompare(b[1]))) {
    sourceSelect.appendChild(el("option", { value: sourceID, text: name }));
  }
  const platformAssets = document.getElementById("platform-assets");
  clear(platformAssets);
  platformAssets.hidden = !platform.assets?.length;
  if (platform.assets?.length) {
    platformAssets.appendChild(el("h3", { text: `${platform.name} artwork` }));
    for (const asset of platform.assets) {
      platformAssets.appendChild(el("button", {
        type: "button",
        class: "platform-asset",
        onclick: () => openPreview(asset, platform.name),
        "aria-label": `Preview ${platform.name} ${asset.role}`,
      }, [image(asset.id, `${platform.name} ${asset.role}`), chip(asset.role)]));
    }
  }
  renderGameGrid();
  showView("platform-detail");
}

function chip(text, className = "") {
  return el("span", { class: `chip ${className}`.trim(), text });
}

function openPreview(asset, title) {
  const dialog = document.getElementById("artwork-preview");
  dialogOpeners.set(dialog, document.activeElement);
  const figure = dialog.querySelector("figure");
  clear(figure);
  figure.appendChild(image(asset.id, `${title} ${asset.role}`, "preview-image"));
  figure.appendChild(el("figcaption", { text: `${title} · ${asset.role} · ${asset.width || "?"}x${asset.height || "?"}` }));
  dialog.showModal();
}

async function chooseForProfile(asset, game, api, announceStatus, announceError) {
  const summaries = await api.get("/api/review/profiles");
  if (!summaries.length) {
    announceError("Create a profile in Advanced before choosing artwork for it.");
    document.getElementById("advanced").open = true;
    document.getElementById("profiles")?.scrollIntoView();
    return;
  }
  const dialog = document.getElementById("profile-picker");
  dialogOpeners.set(dialog, document.activeElement);
  const select = dialog.querySelector("select");
  clear(select);
  for (const profile of summaries) select.appendChild(el("option", { value: profile.id, text: profile.name }));
  dialog.querySelector("[data-role]").textContent = asset.role;
  dialog.returnValue = "";
  dialog.showModal();
  const result = await new Promise((resolve) => dialog.addEventListener("close", () => resolve(dialog.returnValue), { once: true }));
  if (result !== "save") return;
  try {
    const id = select.value;
    const view = await api.get(`/api/drafts/profiles/${encodeURIComponent(id)}`);
    const profile = view.draft.profile;
    let profileGame = profile.games.find((item) => item.id === game.id);
    if (!profileGame) {
      profileGame = { id: game.id, identities: game.identities, assets: {} };
      profile.games.push(profileGame);
    }
    profileGame.assets[asset.role] = { sha256: asset.sha256, extension: asset.extension };
    await api.put(`/api/drafts/profiles/${encodeURIComponent(id)}`, {
      baseDigest: view.draft.digest,
      profile,
    });
    announceStatus(`${asset.role} artwork selected for ${profile.name}.`);
    await renderProfiles(api);
  } catch (err) {
    announceError(`Could not update the profile: ${err.message}`);
  }
}

function assetCard(asset, game, api, announceStatus, announceError) {
  const facts = [
    asset.width && asset.height ? `${asset.width}x${asset.height}` : "Dimensions unknown",
    asset.aspect || "Aspect unknown",
    asset.extension ? asset.extension.toUpperCase() : asset.mime,
    formatBytes(asset.size),
    asset.sourceName,
    asset.location,
  ];
  if (asset.sharedCopies > 1) facts.push(`Copy stored in ${asset.sharedCopies} places`);
  if (asset.profiles?.length) facts.push(`Used by ${asset.profiles.length} profiles`);
  return el("article", { class: "asset-card" }, [
    el("button", {
      type: "button", class: "asset-preview",
      onclick: () => openPreview(asset, game.title),
      "aria-label": `Preview ${game.title} ${asset.role}`,
    }, [image(asset.id, `${game.title} ${asset.role}`)]),
    el("div", { class: "asset-card__body" }, [
      el("h3", { text: asset.role }),
      el("div", { class: "chip-row" }, facts.map((fact) => chip(fact))),
      el("button", {
        type: "button", class: "secondary-action", text: "Choose for a profile",
        onclick: () => chooseForProfile(asset, game, api, announceStatus, announceError),
      }),
    ]),
  ]);
}

function openGame(game) {
  activeGame = game;
  document.getElementById("game-detail-heading").textContent = game.title;
  document.getElementById("game-platform").textContent = game.platformName;
  const identities = document.getElementById("game-identities");
  clear(identities);
  for (const [namespace, value] of Object.entries(game.identities || {})) identities.appendChild(chip(`${namespace}: ${value}`));
  document.getElementById("game-profile-use").textContent = game.profiles?.length
    ? `Used by ${game.profiles.join(", ")}`
    : "Not used by a saved profile";
  const missing = document.getElementById("missing-artwork");
  clear(missing);
  if (game.missingRoles.length) {
    missing.appendChild(el("h3", { text: "Missing artwork" }));
    for (const fallback of game.fallbacks || []) {
      missing.appendChild(el("p", {
        text: `${fallback.message} Missing: ${fallback.roles.join(", ")}.`,
      }));
    }
    missing.appendChild(el("div", { class: "chip-row" }, game.missingRoles.map((role) => chip(role, "chip--missing"))));
  }
  const gallery = document.getElementById("asset-gallery");
  clear(gallery);
  for (const asset of game.assets) gallery.appendChild(assetCard(asset, game, window.gamelibAPI, window.announceStatus, window.announceError));
  showView("game-detail");
}

async function renderProfiles(api) {
  const grid = document.getElementById("profile-card-grid");
  clear(grid);
  const profiles = await api.get("/api/review/profiles");
  if (!profiles.length) {
    grid.appendChild(el("p", { class: "empty-state", text: "No saved profiles yet. Create one in Advanced, then choose artwork directly from a game's gallery." }));
    return;
  }
  for (const profile of profiles) {
    const view = await api.get(`/api/drafts/profiles/${encodeURIComponent(profile.id)}`);
    const games = view.draft?.profile?.games || [];
    const assets = games.flatMap((game) => Object.values(game.assets || {}));
    grid.appendChild(el("article", { class: "profile-card" }, [
      el("div", { class: "profile-mosaic" }, assets.slice(0, 4).map((asset) => {
        const found = catalog?.games.flatMap((game) => game.assets).find((item) => item.sha256 === asset.sha256);
        return found ? image(found.id, "") : el("span", { class: "mosaic-placeholder" });
      })),
      el("div", {}, [
        el("h3", { text: profile.name }),
        el("p", { text: `${games.length} games · ${assets.length} artwork selections` }),
        profile.theme ? chip(`Theme: ${profile.theme}`) : null,
        el("a", {
          href: "#profiles", class: "text-link", text: "Edit profile details in Advanced",
          "data-advanced-target": "profiles",
        }),
      ]),
    ]));
  }
}

async function renderSources(api) {
  const grid = document.getElementById("source-card-grid");
  clear(grid);
  const [detected, config] = await Promise.all([
    api.get("/api/sources/detect"),
    api.get("/api/config"),
  ]);
  const pathKey = (path) => detected.caseSensitive ? path : path.toLocaleLowerCase();
  const detectedByPath = new Map((detected.sources || []).map((item) => [pathKey(item.path), item]));
  const roots = config.config?.roots || [];
  const items = [...detected.sources || []];
  for (const root of roots) {
    if (!detectedByPath.has(pathKey(root.path))) {
      items.push({ ...root, name: root.id, itemCount: null, configured: true });
    }
  }
  if (!items.length) {
    grid.appendChild(el("p", { class: "empty-state", text: "No artwork sources were detected. Add a folder manually in Advanced." }));
    return detected;
  }
  for (const source of items) {
    grid.appendChild(el("article", { class: "source-card" }, [
      el("div", {}, [
        el("h3", { text: source.name }),
        el("p", { text: source.itemCount === null ? "Configured source" : `${source.itemCount} items found` }),
        source.configured && catalog?.scannedAt
          ? el("small", { text: `Last scan ${new Date(catalog.scannedAt).toLocaleString()}` })
          : null,
      ]),
      el("div", { class: "source-actions" }, [
        chip(source.configured ? "Connected" : "Detected", source.configured ? "chip--connected" : ""),
        source.configured ? el("button", {
          type: "button", class: "secondary-action", text: "Rescan",
          onclick: () => document.getElementById("library-refresh").click(),
        }) : null,
      ]),
    ]));
  }
  return detected;
}

async function setupOnboarding(api, announceStatus, announceError) {
  const config = await api.get("/api/config");
  if (config.exists) return false;
  const detected = await api.get("/api/sources/detect");
  const onboarding = document.getElementById("onboarding");
  const list = document.getElementById("detected-sources-list");
  onboarding.hidden = false;
  clear(list);
  for (const source of detected.sources || []) {
    const checkbox = el("input", { type: "checkbox", value: source.id, checked: true });
    list.appendChild(el("label", { class: "source-option" }, [
      checkbox,
      el("span", {}, [
        el("strong", { text: source.name }),
        el("small", { text: `${source.itemCount} items` }),
      ]),
    ]));
  }
  if (!(detected.sources || []).length) {
    list.appendChild(el("p", { class: "empty-state", text: "No common artwork locations were found. Add a folder manually below." }));
  }
  document.getElementById("detected-sources-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const selected = new Set([...list.querySelectorAll("input:checked")].map((input) => input.value));
    const roots = (detected.sources || []).filter((source) => selected.has(source.id)).map(({ id, kind, path, system }) => ({ id, kind, path, system }));
    if (!roots.length) {
      announceError("Select at least one detected source, or add a folder manually.");
      return;
    }
    try {
      await api.put("/api/config", {
        baseDigest: "",
        config: { version: 1, roots, policy: { version: 1, default: "tracked-external", rules: [] } },
      });
      onboarding.hidden = true;
      announceStatus("Sources added. Scanning artwork now.");
      await loadCatalog(api, announceError);
    } catch (err) {
      announceError(`Could not add sources: ${err.message}`);
    }
  });
  return true;
}

async function loadCatalog(api, announceError, monitorScan = true) {
  try {
    catalog = await api.get("/api/organizer");
    renderLibrary();
    await renderProfiles(api);
    if (monitorScan) {
      const status = await api.get("/api/organizer/scan");
      if (status.status === "scanning") void pollScan(api, announceError);
      else if (status.status === "complete" && status.total > 0 && catalog.games.length === 0) {
        await loadCatalog(api, announceError, false);
      }
    }
  } catch (err) {
    if (err.code !== "config_missing") announceError(`Could not load the library: ${err.message}`);
  }
}

async function pollScan(api, announceError) {
  if (scanPolling) return;
  scanPolling = true;
  const progress = document.getElementById("scan-progress");
  progress.hidden = false;
  let lastCompleted = -1;
  try {
    while (true) {
      const status = await api.get("/api/organizer/scan");
      const active = status.roots?.find((root) => root.status === "scanning");
      progress.textContent = active
        ? `Scanning ${active.id} · ${status.completed} of ${status.total} sources complete`
        : `${status.completed} of ${status.total} sources complete`;
      if (status.completed !== lastCompleted) {
        lastCompleted = status.completed;
        await loadCatalog(api, announceError, false);
      }
      if (status.status === "idle") {
        await loadCatalog(api, announceError, false);
      } else if (status.status === "complete") {
        progress.textContent = `Scan complete · ${status.completed} sources checked`;
        break;
      }
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
  } catch (err) {
    announceError(`Artwork scan failed: ${err.message}`);
  } finally {
    scanPolling = false;
  }
}

export async function init({ api, announceStatus, announceError }) {
  window.gamelibAPI = api;
  window.announceStatus = announceStatus;
  window.announceError = announceError;
  const onboarding = await setupOnboarding(api, announceStatus, announceError);
  if (!onboarding) await loadCatalog(api, announceError);
  await renderSources(api);
  showView("library", false);

  document.getElementById("platform-back").addEventListener("click", () => showView("library"));
  document.getElementById("game-back").addEventListener("click", () => openPlatform(activePlatform));
  for (const id of ["game-search", "game-coverage", "game-role", "game-source", "game-sort"]) {
    document.getElementById(id).addEventListener(id === "game-search" ? "input" : "change", renderGameGrid);
  }
  document.getElementById("library-refresh").addEventListener("click", async () => {
    try {
      const progress = document.getElementById("scan-progress");
      progress.hidden = false;
      progress.textContent = "Starting artwork scan...";
      await api.post("/api/organizer/scan");
      await pollScan(api, announceError);
      announceStatus("Artwork scan complete.");
    } catch (err) {
      announceError(`Artwork scan failed: ${err.message}`);
    }
  });
  document.getElementById("sources-detect").addEventListener("click", () => renderSources(api));
  document.getElementById("preview-close").addEventListener("click", () => document.getElementById("artwork-preview").close());
  for (const dialog of document.querySelectorAll("dialog")) {
    dialog.addEventListener("close", () => {
      const opener = dialogOpeners.get(dialog);
      if (opener?.isConnected) opener.focus();
      dialogOpeners.delete(dialog);
    });
  }
  for (const link of document.querySelectorAll(".nav-primary")) {
    link.addEventListener("click", (event) => {
      event.preventDefault();
      showView(link.getAttribute("href").slice(1));
    });
  }
  const advancedLink = document.querySelector('.stage-rail__list a[href="#advanced"]');
  advancedLink.addEventListener("click", (event) => {
    event.preventDefault();
    for (const view of document.querySelectorAll(".organizer-view")) view.hidden = true;
    for (const link of document.querySelectorAll(".nav-primary")) link.removeAttribute("aria-current");
    const advanced = document.getElementById("advanced");
    advanced.open = true;
    advanced.scrollIntoView({ block: "start" });
    advanced.querySelector("summary").focus();
  });
  document.addEventListener("click", (event) => {
    const link = event.target.closest("[data-advanced-target]");
    if (!link) return;
    event.preventDefault();
    for (const view of document.querySelectorAll(".organizer-view")) view.hidden = true;
    const target = document.getElementById(link.dataset.advancedTarget);
    const advanced = document.getElementById("advanced");
    advanced.open = true;
    target?.scrollIntoView({ block: "start" });
    const heading = target?.querySelector("h2");
    if (heading) {
      heading.setAttribute("tabindex", "-1");
      heading.focus({ preventScroll: true });
    }
  });
}
