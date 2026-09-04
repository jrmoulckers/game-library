// main.js: the dashboard's single entry point (loaded as a native ES
// module, no bundler/framework). It fetches bootstrap info once, then
// hands each <section> to its own module to progressively enhance. A
// failure in one section is reported through the shared error live region
// without stopping the others from initializing.

import { api, configureCSRF } from './api.js';
import { initStatus, announceError } from './status.js';
import * as setup from './setup.js';
import * as organizer from './organizer.js';

// Only the library organizer and the source setup form remain. The
// former review/audit sections (overview, artwork, identity,
// duplicates, policy, profiles, plans, adapters, recovery) each ran
// their own API calls on page load, which dominated startup cost.
const sections = [['setup', setup]];

function initStageRail() {
  const toggle = document.getElementById('stage-rail-toggle');
  const list = document.getElementById('stage-rail-list');
  if (!toggle || !list) return;
  toggle.addEventListener('click', () => {
    const expanded = toggle.getAttribute('aria-expanded') === 'true';
    toggle.setAttribute('aria-expanded', String(!expanded));
    list.hidden = expanded;
  });
}

async function main() {
  initStatus();
  initStageRail();

  try {
    const boot = await api.get('/api/bootstrap');
    configureCSRF(boot.csrfToken, boot.csrfHeader);
    const context = document.getElementById('workspace-context');
    if (context) {
      context.textContent = `Local workspace: ${boot.workspaceRoot}. Active configuration file: ${boot.configPath}.`;
    }
    const setupPath = document.getElementById('setup-config-path');
    if (setupPath) setupPath.textContent = boot.configPath;
  } catch (err) {
    announceError(
      `Failed to reach the dashboard server: ${err.message}. Confirm "gamelib serve" is still running.`,
    );
    return;
  }

  try {
    await organizer.init({ api, announceStatus: announceStatusSafe, announceError });
  } catch (err) {
    announceError(`Failed to initialize the artwork organizer: ${err.message}`);
  }

  for (const [name, module] of sections) {
    const sectionRoot = document.getElementById(name);
    if (!sectionRoot || typeof module.init !== 'function') continue;
    try {
      await module.init(sectionRoot, { api, announceStatus: announceStatusSafe, announceError });
    } catch (err) {
      announceError(`Failed to initialize the "${name}" section: ${err.message}`);
    }
  }
}

// announceStatusSafe is a thin indirection so a circular import isn't
// needed between main.js and status.js for the common case of a section
// module calling both announceStatus and announceError.
function announceStatusSafe(message) {
  const region = document.getElementById('status-region');
  if (region) region.textContent = message;
}

main();
