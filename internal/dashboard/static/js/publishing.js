import { clear, el } from './dom.js';

let api;
let profileKey = '';
let generation = 0;
let targetsConfig = null;
let busy = false;

function status(message) {
  document.getElementById('publishing-status').textContent = message;
}

async function action(button, work) {
  if (busy) return;
  busy = true;
  button.disabled = true;
  try {
    await work();
  } catch (err) {
    status(`${err.message} No further copies were attempted. Compare again before retrying.`);
    window.announceError(err.message);
  } finally {
    busy = false;
    button.disabled = false;
  }
}

function renderFiles(plan) {
  const body = el('tbody');
  for (const file of plan.files) {
    body.appendChild(
      el('tr', {}, [
        el('td', {}, [el('code', { text: file.path })]),
        el('td', {
          text: file.blocked
            ? `Excluded: ${file.reason}`
            : file.status === 'matching'
              ? 'Keep matching file'
              : file.status === 'different'
                ? 'Back up and replace'
                : file.status === 'missing'
                  ? 'Add copy'
                  : 'Not observed',
        }),
        el('td', {}, [el('code', { text: file.previous || 'Absent / not observed' })]),
        el('td', {}, [el('code', { text: file.sha256 })]),
      ]),
    );
  }
  return el(
    'div',
    {
      class: 'table-scroller publishing-files',
      tabindex: '0',
      role: 'region',
      'aria-label': 'Exact export file preview',
    },
    [
      el('table', {}, [
        el('caption', {
          text: `${plan.files.length} compared files · ${plan.blocked || 0} blocked from copying · SHA-256 before and after`,
        }),
        el('thead', {}, [
          el(
            'tr',
            {},
            ['File', 'Action', 'Current SHA-256', 'Export SHA-256'].map((text) =>
              el('th', { scope: 'col', text }),
            ),
          ),
        ]),
        body,
      ]),
    ],
  );
}

function showPreview(view, rollback = false) {
  const host = document.getElementById('publishing-preview');
  clear(host);
  host.hidden = false;
  const { plan, target } = view;
  host.append(
    el('h4', { text: rollback ? 'Review backup restoration' : 'Review exact artwork copies' }),
    el('p', { text: `Target: ${target.name}` }),
    el('p', { class: 'publishing-path' }, [
      el('code', { text: target.root || 'Unobserved: no frontend folder configured' }),
    ]),
    el('p', {
      text: rollback
        ? `${view.retained} added files will be retained. Nothing is deleted.`
        : `No deletes. Matching files stay untouched. Replacements are backed up before copying. ${plan.blocked || 0} unsupported files are excluded, not converted or renamed.`,
    }),
    renderFiles(plan),
    el('p', { class: 'publishing-path' }, [
      el('span', { text: 'Preview lock: ' }),
      el('code', { text: plan.digest }),
    ]),
  );
  if (
    !rollback &&
    plan.state === 'observed' &&
    plan.publishable.files > 0 &&
    plan.publishable.matching === plan.publishable.files
  ) {
    host.appendChild(
      el('p', {
        text: `All ${plan.publishable.files} publishable files already match. This preview requires zero frontend writes; excluded files are not repaired or removed.`,
      }),
    );
  }
  if (!rollback && plan.state !== 'unsupported' && plan.blocked !== plan.files.length) {
    const stage = el('button', { type: 'button', text: 'Generate local export' });
    stage.addEventListener('click', () =>
      action(stage, async () => {
        const result = await api.post('/api/publishing/stage', {
          profile: plan.profile,
          target: target.id,
          approval: plan.digest,
        });
        status(
          `Export staged in the local workspace: ${result.path}. No frontend artwork changed.`,
        );
      }),
    );
    host.appendChild(stage);
  }
  if (plan.state !== 'observed' || (!rollback && plan.blocked === plan.files.length)) {
    host.appendChild(
      el('p', {
        text:
          plan.message || 'This target has not been safely observed. Publishing is unavailable.',
      }),
    );
    return;
  }
  const checkbox = el('input', { type: 'checkbox' });
  const approve = el('button', {
    type: 'button',
    class: 'button-primary',
    disabled: true,
    text: rollback ? 'Approve restoration' : 'Approve these copies',
  });
  checkbox.addEventListener('change', () => {
    approve.disabled = !checkbox.checked;
  });
  host.append(
    el('label', { class: 'publishing-approval' }, [
      checkbox,
      el('span', {
        text: `I approve only the exact ${rollback ? 'restore' : 'copy'} file set and target folder shown above.`,
      }),
    ]),
    approve,
  );
  approve.addEventListener('click', () =>
    action(approve, async () => {
      const endpoint = rollback ? '/api/publishing/rollback' : '/api/publishing/publish';
      const result = await api.post(endpoint, {
        profile: plan.profile,
        target: target.id,
        approval: plan.digest,
        ...(rollback ? { operation: view.operation } : {}),
      });
      clear(host);
      host.hidden = true;
      await refresh();
      status(
        `${rollback ? 'Backup restoration' : 'Publishing'} ${result.state}. No artwork was deleted.`,
      );
    }),
  );
  host.scrollIntoView({ block: 'nearest' });
}

async function renderHistory(key, revision) {
  const receipts = await api.get('/api/publishing/history');
  if (revision !== generation) return;
  const host = document.getElementById('publishing-history');
  clear(host);
  const entries = receipts.filter((receipt) => receipt.plan.profile === key);
  if (!entries.length)
    host.appendChild(el('p', { text: 'No artwork has been published from this workspace.' }));
  for (const receipt of entries) {
    const row = el('div', { class: 'publishing-row' }, [
      el('p', {
        text: `${receipt.plan.target} · ${receipt.state} · ${receipt.prepared.length} copies prepared`,
      }),
      el('code', { text: receipt.plan.digest }),
    ]);
    if (receipt.state !== 'rolled-back') {
      const button = el('button', { type: 'button', text: 'Preview rollback' });
      button.addEventListener('click', () =>
        action(button, async () => {
          const view = await api.post('/api/publishing/rollback-preview', {
            target: receipt.plan.target,
            operation: receipt.plan.digest,
          });
          if (profileKey === key) showPreview(view, true);
        }),
      );
      row.appendChild(button);
    }
    host.appendChild(row);
  }
}

function renderConfig(config) {
  targetsConfig = config;
  document.getElementById('publishing-target-config').querySelector('button').disabled = false;
  const host = document.getElementById('publishing-target-fields');
  clear(host);
  for (const target of config.targets) {
    const input = el('input', {
      type: 'text',
      value: target.root || '',
      placeholder: 'Leave empty if unobserved',
      'data-target-id': target.id,
    });
    const frontend = el('input', {
      type: 'text',
      value: target.adapter || '',
      placeholder: 'Unknown frontend',
      'data-adapter-id': target.id,
    });
    host.appendChild(
      el('fieldset', {}, [
        el('legend', { text: target.name }),
        el('label', {}, [
          el('span', { text: 'Observed frontend adapter (steam supported; others not enabled)' }),
          frontend,
        ]),
        el('label', {}, [el('span', { text: 'Actual folder on this host' }), input]),
      ]),
    );
  }
}

async function refresh() {
  const revision = ++generation;
  const key = profileKey;
  status('Comparing current artwork bytes...');
  const [report, config] = await Promise.all([
    api.get(`/api/publishing/parity?profile=${encodeURIComponent(key)}`),
    api.get('/api/publishing/targets'),
  ]);
  if (revision !== generation) return;
  clear(document.getElementById('publishing-preview'));
  document.getElementById('publishing-preview').hidden = true;
  renderConfig(config);
  const host = document.getElementById('publishing-targets');
  clear(host);
  for (const plan of report.plans) {
    const target = report.targets.find((item) => item.id === plan.target);
    const row = el('div', { class: 'publishing-row' }, [
      el('h4', { text: target?.name || plan.target }),
      el('p', {
        text:
          plan.state === 'observed'
            ? `${plan.matching} matching · ${plan.missing} missing · ${plan.different} different · ${plan.unknown} unknown · ${plan.blocked} blocked from copying`
            : plan.state,
      }),
      plan.message ? el('p', { class: 'context-line', text: plan.message }) : null,
      plan.publishable && plan.files.length
        ? el('p', {
            text: `Publishable subset: ${plan.publishable.files} files · ${plan.publishable.matching} matching · ${plan.publishable.missing} missing · ${plan.publishable.different} different · ${plan.publishable.unknown} unknown`,
          })
        : null,
    ]);
    if (plan.digest) {
      const preview = el('button', { type: 'button', text: 'Preview export' });
      preview.addEventListener('click', () =>
        action(preview, async () => {
          const view = await api.post('/api/publishing/preview', {
            profile: key,
            target: plan.target,
          });
          if (profileKey === key) showPreview(view);
        }),
      );
      row.appendChild(preview);
    }
    host.appendChild(row);
  }
  if (!report.plans.length)
    host.appendChild(el('p', { text: 'No frontend targets are declared for this platform.' }));
  await renderHistory(key, revision);
  if (revision === generation)
    status(
      'Comparison complete. Only available files were measured; unavailable targets are not verified.',
    );
}

export function openPublishing(key) {
  profileKey = key;
  clear(document.getElementById('publishing-targets'));
  clear(document.getElementById('publishing-preview'));
  clear(document.getElementById('publishing-history'));
  clear(document.getElementById('publishing-target-fields'));
  document.getElementById('publishing-target-config').querySelector('button').disabled = true;
  refresh().catch((err) => status(`Could not compare artwork: ${err.message}`));
}

export function initPublishing(client) {
  api = client;
  const button = document.getElementById('publishing-refresh');
  button.addEventListener('click', () => action(button, refresh));
  const form = document.getElementById('publishing-target-config');
  form.addEventListener('submit', (event) => {
    event.preventDefault();
    const save = form.querySelector('button');
    action(save, async () => {
      const targets = targetsConfig.targets.map((target) => ({
        ...target,
        root: form.querySelector(`[data-target-id="${CSS.escape(target.id)}"]`).value.trim(),
        adapter: form.querySelector(`[data-adapter-id="${CSS.escape(target.id)}"]`).value.trim(),
      }));
      await api.put('/api/publishing/targets', {
        baseDigest: targetsConfig.digest,
        targets: { version: 1, targets },
      });
      await refresh();
      status('Frontend labels saved locally. No artwork changed; these labels are not synced.');
    });
  });
}
