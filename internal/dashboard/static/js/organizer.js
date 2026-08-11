import { clear, el, formatBytes } from "./dom.js";

let catalog = null;
let coverage = null;
let activePlatform = "";
let activeGame = null;
let activeProfile = null;
let scanPolling = false;
let metadataPolling = false;
let lastMetadataStatus = "";
const dialogOpeners = new WeakMap();

function mediaURL(id) {
  return `/api/review/media/${encodeURIComponent(id)}`;
}

function thumbURL(id) {
  return `/api/media/${encodeURIComponent(id)}/thumb`;
}

function image(id, alt, className = "") {
  const img = el("img", { src: thumbURL(id), alt, loading: "lazy", decoding: "async", class: className });
  img.addEventListener("error", () => {
    img.hidden = true;
    const fallback = el("span", { class: "broken-art", text: "Preview unavailable" });
    img.parentElement?.appendChild(fallback);
  }, { once: true });
  return img;
}

function fullImage(id, alt, className = "") {
  const img = el("img", { src: mediaURL(id), alt, loading: "lazy", decoding: "async", class: className });
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
  if (catalog.metadataStatus === "loading") summary.append(" Resolving local game titles...");
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

// Tiles are built once per platform and then shown/hidden in place.
// Rebuilding the grid on every keystroke recreated every <img>, which
// made typing in the search box unusable on a real library.
let tileNodes = new Map();
let tilePlatform = null;

function ensureTiles() {
  const grid = document.getElementById("game-grid");
  if (tilePlatform === activePlatform && tileNodes.size) return grid;
  clear(grid);
  tileNodes = new Map();
  tilePlatform = activePlatform;
  const fragment = document.createDocumentFragment();
  for (const game of catalog.games || []) {
    if (game.platformId !== activePlatform) continue;
    const node = gameTile(game);
    tileNodes.set(game.id, node);
    fragment.appendChild(node);
  }
  grid.appendChild(fragment);
  return grid;
}

function renderGameGrid() {
  const grid = ensureTiles();
  const games = visibleGames();
  const order = new Map(games.map((game, index) => [game.id, index]));
  for (const [id, node] of tileNodes) {
    const index = order.get(id);
    if (index === undefined) {
      node.hidden = true;
      continue;
    }
    node.hidden = false;
    // Reordering via CSS order avoids DOM moves entirely, so images
    // are never detached and never re-requested while filtering.
    node.style.order = String(index);
  }
  let empty = grid.querySelector(".empty-state");
  if (!games.length) {
    if (!empty) grid.appendChild(el("p", { class: "empty-state", text: "No games match these filters." }));
    else empty.hidden = false;
    document.getElementById("game-grid-status").textContent = "No games match these filters.";
    return;
  }
  if (empty) empty.hidden = true;
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
  figure.appendChild(fullImage(asset.id, `${title} ${asset.role}`, "preview-image"));
  figure.appendChild(el("figcaption", { text: `${title} · ${asset.role} · ${asset.width || "?"}x${asset.height || "?"}` }));
  dialog.showModal();
}

// profileForArtworkSet names the profile a catalog artwork set belongs
// to, so a file on disk can say which profile it is part of.
function profileForArtworkSet(set) {
  if (!set || !coverage) return null;
  return coverage.profiles.find((profile) => profile.artworkSet === set) || null;
}

function assetCard(asset, game) {
  const facts = [
    asset.width && asset.height ? `${asset.width}x${asset.height}` : "Dimensions unknown",
    asset.aspect || "Aspect unknown",
    asset.extension ? asset.extension.toUpperCase() : asset.mime,
    formatBytes(asset.size),
    asset.sourceName,
  ];
  if (asset.sharedCopies > 1) facts.push(`Stored in ${asset.sharedCopies} places`);
  const owner = profileForArtworkSet(asset.artworkSet);
  const belonging = owner
    ? chip(`${owner.platformName} · ${owner.name}`, "chip--profile")
    : asset.artworkSet
      ? chip(`Unassigned set: ${asset.artworkSet}`, "chip--missing")
      : chip(`Live folder · ${asset.sourceId}`, "chip--live");
  return el("article", { class: "asset-card" }, [
    el("button", {
      type: "button", class: "asset-preview",
      onclick: () => openPreview(asset, game.title),
      "aria-label": `Preview ${game.title} ${asset.role}`,
    }, [image(asset.id, `${game.title} ${asset.role}`)]),
    el("div", { class: "asset-card__body" }, [
      el("h3", { text: asset.role }),
      el("div", { class: "chip-row" }, [belonging]),
      el("div", { class: "chip-row" }, facts.map((fact) => chip(fact))),
      el("p", { class: "asset-location", text: asset.location }),
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
  if (game.store) identities.appendChild(chip(`Library: ${game.store}`));
  renderGameCoverage(game);
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
  for (const asset of game.assets) gallery.appendChild(assetCard(asset, game));
  showView("game-detail");
}

// renderGameCoverage answers "which profiles have media for this game",
// including the profiles that apply to this game's platform but hold
// nothing for it. The gaps are the point.
function renderGameCoverage(game) {
  const panel = document.getElementById("game-profile-coverage");
  const list = document.getElementById("game-profile-coverage-list");
  const summary = document.getElementById("game-profile-coverage-summary");
  const entry = coverage?.games.find((item) => item.gameId === game.id);
  clear(list);
  if (!entry || !entry.profiles.length) {
    panel.hidden = true;
    return;
  }
  panel.hidden = false;
  summary.textContent = `${entry.coveredCount} of ${entry.profiles.length} ${entry.platformName} profiles have artwork for this game.`;
  for (const profile of entry.profiles) {
    list.appendChild(el("li", { class: profile.covered ? "coverage-row" : "coverage-row coverage-row--gap" }, [
      el("button", {
        type: "button", class: "coverage-row__name",
        onclick: () => openProfile(profile.key),
      }, [
        el("strong", { text: profile.name }),
        el("span", { class: "coverage-row__platform", text: profile.platformName }),
      ]),
      el("div", { class: "chip-row" }, profile.covered
        ? profile.roles.map((role) => chip(role, "chip--role"))
        : [chip("no artwork here", "chip--missing")]),
      el("div", { class: "chip-row coverage-row__devices" },
        profile.devices.map((device) => chip(device.name, "chip--device"))),
    ]));
  }
}

function deviceLine(devices) {
  if (!devices.length) return "Not on any declared device";
  return `On ${devices.map((device) => device.name).join(", ")}`;
}

function profileCard(profile) {
  const body = [
    el("h3", { text: profile.name }),
    el("p", {
      text: profile.empty
        ? "No artwork yet"
        : `${profile.gameCount} ${profile.gameCount === 1 ? "game" : "games"} · ${profile.assetCount} files`,
    }),
    el("div", { class: "chip-row" }, profile.devices.map((device) => chip(device.name, "chip--device"))),
  ];
  if (!profile.empty) {
    body.push(el("div", { class: "chip-row" },
      profile.roles.slice(0, 6).map((role) => chip(`${role.role} ${role.count}`, "chip--role"))));
    body.push(el("p", { class: "profile-note", text: `Artwork set: ${profile.artworkSet}` }));
  }
  return el("article", { class: profile.empty ? "profile-card profile-card--empty" : "profile-card" }, [
    el("button", {
      type: "button", class: "profile-card__open",
      onclick: () => openProfile(profile.key),
      "aria-label": `Open ${profile.platformName} ${profile.name}`,
    }, body),
  ]);
}

// renderProfiles groups profiles under the platform they belong to,
// because a profile name only means something alongside its platform.
async function renderProfiles(api) {
  const host = document.getElementById("profile-platforms");
  clear(host);
  if (!coverage) await loadCoverage(api);
  const summary = document.getElementById("profile-library-summary");
  const withArt = coverage.profiles.filter((profile) => !profile.empty).length;
  summary.textContent = `${coverage.profiles.length} profiles across ${new Set(coverage.profiles.map((p) => p.platformId)).size} platforms · ${withArt} currently hold artwork. A profile is stored once and reaches every device that runs its platform.`;

  const byPlatform = new Map();
  for (const profile of coverage.profiles) {
    if (!byPlatform.has(profile.platformId)) byPlatform.set(profile.platformId, []);
    byPlatform.get(profile.platformId).push(profile);
  }
  for (const [platformId, profiles] of byPlatform) {
    const devices = profiles[0]?.devices || [];
    host.appendChild(el("section", { class: "platform-group" }, [
      el("header", { class: "platform-group__head" }, [
        el("h3", { text: profiles[0]?.platformName || platformId }),
        el("p", { class: "context-line", text: deviceLine(devices) }),
      ]),
      el("div", { class: "profile-grid" }, profiles.map((profile) => profileCard(profile))),
    ]));
  }
  renderUnbound(api);
  renderLiveSurfaces();
}

// renderUnbound surfaces artwork that exists on disk but that no profile
// claims. Assigning one is a label change in gamelib's own file; no
// artwork is copied, moved, or renamed.
function renderUnbound(api) {
  const panel = document.getElementById("unbound-artwork");
  const list = document.getElementById("unbound-list");
  clear(list);
  if (!coverage.unbound.length) {
    panel.hidden = true;
    return;
  }
  panel.hidden = false;
  for (const set of coverage.unbound) {
    const select = el("select", { "aria-label": `Assign ${set.artworkSet} to a profile` }, [
      el("option", { value: "", text: "Choose a profile…" }),
      ...coverage.profiles
        .filter((profile) => profile.empty)
        .map((profile) => el("option", {
          value: profile.key,
          text: `${profile.platformName} · ${profile.name}`,
        })),
    ]);
    list.appendChild(el("article", { class: "unbound-card" }, [
      el("div", {}, [
        el("h4", { text: set.artworkSet }),
        el("p", { text: `${set.gameCount} games · ${set.assetCount} files` }),
      ]),
      el("div", { class: "unbound-card__actions" }, [
        select,
        el("button", {
          type: "button", class: "button-primary", text: "Assign",
          onclick: () => assignArtwork(api, set.artworkSet, select.value),
        }),
      ]),
    ]));
  }
}

function renderLiveSurfaces() {
  const list = document.getElementById("live-surface-list");
  clear(list);
  for (const surface of coverage.liveSurfaces) {
    list.appendChild(el("article", { class: "source-card" }, [
      el("h3", { text: surface.sourceId }),
      el("p", { text: `${surface.gameCount} games · ${surface.assetCount} files` }),
      el("div", { class: "chip-row" }, [chip(surface.rootKind, "chip--live")]),
    ]));
  }
  if (!coverage.liveSurfaces.length) {
    list.appendChild(el("p", { class: "empty-state", text: "No live frontend folders are configured on this machine." }));
  }
}

// assignArtwork records which profile an existing artwork set belongs to.
// This only edits gamelib's own topology file; the synced catalog and
// every artwork file are left exactly as they are.
async function assignArtwork(api, artworkSet, key) {
  if (!key) {
    window.announceError("Choose which profile this artwork set belongs to.");
    return;
  }
  try {
    const doc = await api.get("/api/topology");
    const target = doc.profiles.find((profile) => `${profile.platform}/${slugify(profile.name)}` === key);
    if (!target) {
      window.announceError("That profile no longer exists. Refresh and try again.");
      return;
    }
    target.artwork = artworkSet;
    await api.put("/api/topology", stripSaved(doc));
    coverage = null;
    await renderProfiles(api);
    window.announceStatus(`"${artworkSet}" is now the artwork for ${target.platform} · ${target.name}. No files were changed.`);
  } catch (err) {
    window.announceError(`Could not assign the artwork set: ${err.message}`);
  }
}

// stripSaved removes the read-only flag the API adds, which the PUT
// endpoint rejects as an unknown field.
function stripSaved(doc) {
  const { saved, ...rest } = doc;
  return rest;
}

function openProfile(key) {
  const profile = coverage?.profiles.find((item) => item.key === key);
  if (!profile) return;
  activeProfile = profile;
  document.getElementById("profile-detail-heading").textContent = profile.name;
  document.getElementById("profile-detail-platform").textContent = profile.platformName;
  const devices = document.getElementById("profile-detail-devices");
  clear(devices);
  for (const device of profile.devices) devices.appendChild(chip(device.name, "chip--device"));
  document.getElementById("profile-detail-summary").textContent = profile.empty
    ? "This profile has no artwork yet. Assign an artwork set to it from the Profiles view."
    : `${profile.gameCount} games · ${profile.assetCount} files · artwork set "${profile.artworkSet}"`;
  document.getElementById("profile-game-search").value = "";
  renderProfileGames();
  showView("profile-detail");
}

// renderProfileGames answers "which games have media in this profile".
function renderProfileGames() {
  const list = document.getElementById("profile-game-list");
  clear(list);
  if (!activeProfile) return;
  const term = document.getElementById("profile-game-search").value.trim().toLowerCase();
  const games = activeProfile.games.filter((game) => !term || game.title.toLowerCase().includes(term));
  document.getElementById("profile-game-status").textContent = `${games.length} games shown`;
  if (!games.length) {
    list.appendChild(el("p", {
      class: "empty-state",
      text: activeProfile.empty
        ? "No artwork is assigned to this profile yet."
        : "No games in this profile match that search.",
    }));
    return;
  }
  for (const game of games) {
    const full = catalog?.games.find((item) => item.id === game.gameId);
    list.appendChild(el("article", { class: "coverage-card" }, [
      el("button", {
        type: "button", class: "coverage-card__open",
        onclick: () => { if (full) openGame(full); },
        "aria-label": `Open ${game.title}`,
      }, [
        el("h4", { text: game.title }),
        el("div", { class: "chip-row" }, game.roles.map((role) => chip(role, "chip--role"))),
        el("p", { class: "context-line", text: `${game.assetCount} ${game.assetCount === 1 ? "file" : "files"}` }),
      ]),
    ]));
  }
}

async function loadCoverage(api) {
  coverage = await api.get("/api/coverage");
  return coverage;
}

// slugify mirrors the server's profile slug rules so a key built in the
// browser matches the one the server computed.
function slugify(name) {
  return name.trim().toLocaleLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

async function populatePlatformOptions(api) {
  const select = document.getElementById("profile-new-platform");
  clear(select);
  const doc = await api.get("/api/topology");
  for (const platform of doc.platforms) {
    select.appendChild(el("option", { value: platform.id, text: platform.name }));
  }
}

async function createProfile(api, announceStatus, announceError) {
  const input = document.getElementById("profile-new-name");
  const platform = document.getElementById("profile-new-platform").value;
  const name = input.value.trim();
  if (!slugify(name)) {
    announceError("Give the profile a name that contains letters or numbers.");
    return;
  }
  try {
    const doc = await api.get("/api/topology");
    const key = `${platform}/${slugify(name)}`;
    if (doc.profiles.some((profile) => `${profile.platform}/${slugify(profile.name)}` === key)) {
      announceError(`${platform} already has a profile named "${name}".`);
      return;
    }
    doc.profiles.push({ platform, name });
    await api.put("/api/topology", stripSaved(doc));
    input.value = "";
    coverage = null;
    await renderProfiles(api);
    announceStatus(`Added the "${name}" profile for ${platform}.`);
  } catch (err) {
    announceError(`Could not add the profile: ${err.message}`);
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
  const items = [...detected.sources || [], ...detected.supported || []];
  const diagnosticsFor = (kind) => (detected.metadataDiagnostics || []).filter((diagnostic) => {
    if (kind === "steam-grid") return diagnostic.source.startsWith("steam");
    if (kind.startsWith("playnite")) return diagnostic.source.startsWith("playnite");
    if (kind === "esde-media") return diagnostic.source.startsWith("esde");
    return false;
  });
  for (const root of roots) {
    if (!detectedByPath.has(pathKey(root.path))) {
      items.push({
        ...root,
        name: root.id,
        itemCount: null,
        configured: true,
        status: "temporarily-unavailable",
        message: "Configured source is not available on this device.",
      });
    }
  }
  if (!items.length) {
    grid.appendChild(el("p", { class: "empty-state", text: "No artwork sources were detected. Add a folder manually under Source folders." }));
    return detected;
  }
  for (const source of items) {
    const diagnostics = diagnosticsFor(source.kind);
    const unavailable = source.status === "temporarily-unavailable";
    const connected = source.configured && !unavailable;
    grid.appendChild(el("article", { class: "source-card" }, [
      el("div", {}, [
        el("h3", { text: source.name }),
        el("p", { text: source.message || (source.itemCount === null ? "Configured source" : `${source.itemCount} items found`) }),
        diagnostics[0] ? el("small", { text: diagnostics[0].message }) : null,
        source.configured && catalog?.scannedAt
          ? el("small", { text: `Last scan ${new Date(catalog.scannedAt).toLocaleString()}` })
          : null,
      ]),
      el("div", { class: "source-actions" }, [
        chip(
          connected ? "Connected" : (unavailable ? "Unavailable" : (source.status === "not-on-this-device" ? "Not on this device" : "Detected")),
          connected ? "chip--connected" : ((unavailable || source.status === "not-on-this-device") ? "chip--neutral" : ""),
        ),
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
    const metadataCompleted = lastMetadataStatus === "loading" && catalog.metadataStatus === "ready";
    lastMetadataStatus = catalog.metadataStatus || "";
    renderLibrary();
    await renderProfiles(api);
    if (metadataCompleted) await renderSources(api);
    if (catalog.metadataStatus === "loading" && !metadataPolling) {
      metadataPolling = true;
      setTimeout(async () => {
        metadataPolling = false;
        await loadCatalog(api, announceError, false);
      }, 300);
    }
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
        await new Promise((resolve) => setTimeout(resolve, 100));
        const replacement = await api.get("/api/organizer/scan");
        if (replacement.status !== "scanning") {
          progress.textContent = "Scan stopped because the source configuration changed.";
          break;
        }
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
  try {
    await populatePlatformOptions(api);
    await renderProfiles(api);
  } catch (err) {
    announceError(`Could not load profile coverage: ${err.message}`);
  }
  showView("library", false);

  document.getElementById("platform-back").addEventListener("click", () => showView("library"));
  document.getElementById("game-back").addEventListener("click", () => openPlatform(activePlatform));
  document.getElementById("profile-back").addEventListener("click", () => showView("profile-library"));
  document.getElementById("profile-refresh").addEventListener("click", async () => {
    coverage = null;
    try {
      await renderProfiles(api);
      announceStatus("Profile coverage refreshed.");
    } catch (err) {
      announceError(`Could not refresh coverage: ${err.message}`);
    }
  });
  {
    // Coalesce keystrokes so filtering a profile's games stays instant.
    let pending = 0;
    document.getElementById("profile-game-search").addEventListener("input", () => {
      cancelAnimationFrame(pending);
      pending = requestAnimationFrame(renderProfileGames);
    });
  }
  document.getElementById("profile-game-filters").addEventListener("submit", (event) => event.preventDefault());
  for (const id of ["game-search", "game-coverage", "game-role", "game-source", "game-sort"]) {
    const control = document.getElementById(id);
    if (id !== "game-search") {
      control.addEventListener("change", renderGameGrid);
      continue;
    }
    // Coalesce keystrokes so a fast typist triggers one filter pass
    // per pause rather than one per character.
    let pending = 0;
    control.addEventListener("input", () => {
      cancelAnimationFrame(pending);
      pending = requestAnimationFrame(renderGameGrid);
    });
  }
  document.getElementById("profile-create").addEventListener("submit", (event) => {
    event.preventDefault();
    createProfile(api, announceStatus, announceError);
  });
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
