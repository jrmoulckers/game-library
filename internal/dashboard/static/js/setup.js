// setup.js: first-run setup — root editor, root validation, global policy
// default, and the active-configuration save flow (with base-digest
// conflict recovery). This module owns only presentation and client-side
// input assembly; config.Validate/workspace.WriteActiveConfig on the Go
// side remain the single source of truth for what is actually valid.

import { el, clear } from './dom.js';

let schemaVersion = 1;
let baseDigest = '';
let existingPolicy = { version: 1, default: 'tracked-external', rules: [] };
let rootRows = [];
let rowSeq = 0;

function newRow(root) {
  rowSeq += 1;
  return {
    key: rowSeq,
    id: root?.id || '',
    kind: root?.kind || '',
    path: root?.path || '',
    system: root?.system || '',
    validation: null,
  };
}

function renderRoots(tbody) {
  clear(tbody);
  if (rootRows.length === 0) {
    tbody.appendChild(
      el('tr', {}, [
        el('td', {
          colspan: '6',
          text: 'No roots configured yet. Use \u201cAdd root\u201d to add the first one.',
        }),
      ]),
    );
    return;
  }
  rootRows.forEach((row, index) => {
    const idInput = el('input', {
      type: 'text',
      'aria-label': `Root ${index + 1} id`,
      value: row.id,
      oninput: (e) => {
        row.id = e.target.value;
      },
    });
    const kindInput = el('input', {
      type: 'text',
      'aria-label': `Root ${index + 1} kind`,
      value: row.kind,
      oninput: (e) => {
        row.kind = e.target.value;
      },
    });
    const pathInput = el('input', {
      type: 'text',
      'aria-label': `Root ${index + 1} path`,
      value: row.path,
      oninput: (e) => {
        row.path = e.target.value;
      },
    });
    const systemInput = el('input', {
      type: 'text',
      'aria-label': `Root ${index + 1} system`,
      value: row.system,
      oninput: (e) => {
        row.system = e.target.value;
      },
    });
    let validationText = 'Not yet validated';
    if (row.validation) {
      const v = row.validation;
      const parts = [];
      parts.push(v.exists ? 'exists' : 'missing');
      if (v.exists) parts.push(v.isDir ? 'directory' : 'not a directory');
      if (v.exists && v.isDir) parts.push(v.readable ? 'readable' : 'not readable');
      if (v.issues && v.issues.length) parts.push(...v.issues);
      validationText = parts.join('; ');
    }
    const removeButton = el('button', {
      type: 'button',
      text: 'Remove',
      'aria-label': `Remove root ${index + 1}`,
      onclick: () => {
        rootRows = rootRows.filter((r) => r.key !== row.key);
        renderRoots(tbody);
        const nextRow = tbody.querySelectorAll('tr')[Math.min(index, rootRows.length - 1)];
        const nextTarget =
          nextRow?.querySelector('input, button') || document.getElementById('setup-add-root');
        nextTarget?.focus();
      },
    });
    tbody.appendChild(
      el('tr', {}, [
        el('td', {}, [idInput]),
        el('td', {}, [kindInput]),
        el('td', {}, [pathInput]),
        el('td', {}, [systemInput]),
        el('td', { text: validationText }),
        el('td', {}, [removeButton]),
      ]),
    );
  });
}

export async function init(root, { api, announceStatus, announceError }) {
  const configPathEl = document.getElementById('setup-config-path');
  const tbody = root.querySelector('#setup-roots-body');
  const addButton = root.querySelector('#setup-add-root');
  const validateButton = root.querySelector('#setup-validate-roots');
  const validationStatus = root.querySelector('#setup-validation-status');
  const form = root.querySelector('#setup-form');
  const defaultModeSelect = root.querySelector('#setup-default-mode');
  const statusEl = root.querySelector('#setup-status');

  async function loadConfig() {
    const view = await api.get('/api/config');
    baseDigest = view.digest || '';
    if (view.exists && view.config) {
      schemaVersion = view.config.version || schemaVersion;
      rootRows = (view.config.roots || []).map(newRow);
      existingPolicy = view.config.policy || existingPolicy;
      defaultModeSelect.value = existingPolicy.default || 'tracked-external';
      if (configPathEl) configPathEl.textContent = '(see workspace context above)';
    } else {
      rootRows = [];
    }
    renderRoots(tbody);
  }

  try {
    await loadConfig();
  } catch (err) {
    announceError(`First-run setup: failed to load the active configuration (${err.message}).`);
  }

  addButton.addEventListener('click', () => {
    rootRows.push(newRow());
    renderRoots(tbody);
  });

  validateButton.addEventListener('click', async () => {
    try {
      const payload = {
        roots: rootRows.map((r) => ({ id: r.id, kind: r.kind, path: r.path, system: r.system })),
      };
      const result = await api.post('/api/config/validate-roots', payload);
      result.results.forEach((v, index) => {
        if (rootRows[index]) rootRows[index].validation = v;
      });
      renderRoots(tbody);
      const collisions = result.caseCollisions || [];
      validationStatus.textContent =
        collisions.length > 0
          ? `Validated ${result.results.length} root(s). Case-colliding IDs: ${collisions.map((g) => g.join(', ')).join(' | ')}.`
          : `Validated ${result.results.length} root(s). No case-colliding IDs.`;
      announceStatus('Root validation complete.');
    } catch (err) {
      announceError(`Root validation failed: ${err.message}`);
    }
  });

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    const config = {
      version: schemaVersion,
      roots: rootRows.map((r) => ({
        id: r.id,
        kind: r.kind,
        path: r.path,
        system: r.system || undefined,
      })),
      policy: { ...existingPolicy, version: schemaVersion, default: defaultModeSelect.value },
    };
    try {
      const view = await api.put('/api/config', { baseDigest, config });
      baseDigest = view.digest || '';
      existingPolicy = (view.config && view.config.policy) || existingPolicy;
      statusEl.textContent = 'Configuration saved to the local workspace.';
      announceStatus('Active configuration saved.');
    } catch (err) {
      if (err.code === 'config_conflict') {
        statusEl.textContent =
          'Save conflict: the configuration changed elsewhere. Reloading the current version — review it and save again.';
        announceError(
          'Configuration save conflict: reloaded the current version. Review and save again.',
        );
        try {
          await loadConfig();
        } catch (reloadErr) {
          announceError(
            `Failed to reload the active configuration after a conflict: ${reloadErr.message}`,
          );
        }
      } else {
        statusEl.textContent = `Save failed: ${err.message}`;
        announceError(`Failed to save the active configuration: ${err.message}`);
      }
    }
  });
}
