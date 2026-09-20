import {svg, iconOf} from '../dom.js';


// A list of addresses that saves itself: every add or remove posts at once, so
// nothing looks saved that isn't. The Admins panel is one. body wraps the list
// as the route wants it.
function listEditor({rows, status, input, button, url, body, empty, initial}) {
  let list = [...initial];

  async function persist() {
    status.classList.remove('error');
    status.textContent = 'Saving…';
    const res = await fetch(url, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body(list)),
    });
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = await res.text();
      return;
    }
    status.textContent = 'Saved.';
  }

  function render() {
    rows.replaceChildren();
    if (!list.length) {
      const hint = document.createElement('div');
      hint.className = 'hint';
      hint.textContent = empty;
      rows.append(hint);
    }
    for (const email of list) {
      const row = document.createElement('div');
      row.className = 'admin-row';
      const label = document.createElement('div');
      label.className = 'email';
      label.textContent = email;
      const remove = document.createElement('button');
      remove.className = 'link-button';
      remove.type = 'button';
      remove.textContent = 'Remove';
      remove.addEventListener('click', async () => {
        list = list.filter(e => e !== email);
        render();
        await persist();
      });
      row.append(label, remove);
      rows.append(row);
    }
  }

  async function add() {
    const email = input.value.trim().toLowerCase();
    if (!email || list.includes(email)) {
      return;
    }
    list.push(email);
    input.value = '';
    render();
    await persist();
  }

  button.addEventListener('click', add);
  input.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      add();
    }
  });
  render();
}

// The tabs on the left show one panel at a time, as Who?'s admin does.
function initTabs() {
  for (const tab of document.querySelectorAll('.tab')) {
    tab.addEventListener('click', () => showPanel(tab.dataset.panel));
  }
}

function showPanel(name) {
  for (const tab of document.querySelectorAll('.tab')) {
    tab.classList.toggle('active', tab.dataset.panel === name);
  }
  for (const panel of document.querySelectorAll('.panel')) {
    panel.hidden = panel.id !== 'panel-' + name;
  }
}

// The categories as the front page has them, read from its own model.
async function loadCategories() {
  const list = document.querySelector('#category-rows');
  const res = await fetch('/api/apps/model');
  if (!res.ok) {
    list.textContent = 'Could not load the categories.';
    return;
  }
  const model = await res.json();
  list.replaceChildren();
  for (const category of model.categories) {
    const row = document.createElement('div');
    row.className = 'category-row';
    const emoji = document.createElement('div');
    emoji.className = 'emoji';
    emoji.append(svg(iconOf(category)));
    const body = document.createElement('div');
    const title = document.createElement('div');
    title.className = 'title';
    title.textContent = category.title;
    const meta = document.createElement('div');
    meta.className = 'meta';
    if (category.style === 'events') {
      meta.textContent = `Upcoming events from HCA-Team · ${(model.upcoming || []).length} ahead`;
    } else if (category.style === 'apps') {
      meta.textContent = `The community apps · ${(model.apps || []).length} you see`;
    } else {
      const style = category.style === 'cards' ? 'Feature cards' : 'Compact tiles';
      meta.textContent = `${style} · ${category.links.length} link${category.links.length === 1 ? '' : 's'}`;
    }
    body.append(title, meta);
    row.append(emoji, body);
    list.append(row);
  }
}

// The triage queue: every report, and one opened for editing and filing.
const feedback = {rows: [], filter: 'New', canFile: false, repo: '', open: null};

function field(tag, label, value, attrs = {}) {
  const wrap = document.createElement('label');
  wrap.append(label);
  const input = document.createElement(tag);
  Object.assign(input, attrs);
  input.value = value;
  wrap.append(input);
  return {wrap, input};
}

function describe(report) {
  const parts = [report.appName || report.app, report.received].filter(Boolean);
  if (report.status === 'Filed') {
    parts.push(`filed by ${report.handledBy}`);
  } else if (report.status === 'Dismissed') {
    parts.push(`dismissed by ${report.handledBy}`);
  }
  return parts.join(' · ');
}

function renderReports() {
  const list = document.querySelector('#feedback-rows');
  list.replaceChildren();
  const shown = feedback.rows.filter(r => !feedback.filter || r.status === feedback.filter);
  if (!shown.length) {
    const hint = document.createElement('div');
    hint.className = 'hint';
    hint.textContent = feedback.filter === 'New' ? 'Nothing waiting. Everything reported has been filed or dismissed.' : 'Nothing here.';
    list.append(hint);
    return;
  }
  for (const report of shown) {
    const row = document.createElement('div');
    row.className = 'report-row' + (feedback.open === report.id ? ' open' : '');
    const kind = document.createElement('div');
    kind.className = 'kind ' + report.kind;
    kind.textContent = report.kind === 'bug' ? 'Problem' : 'Idea';
    const body = document.createElement('div');
    body.className = 'body';
    const summary = document.createElement('div');
    summary.className = 'summary';
    summary.textContent = report.summary;
    const meta = document.createElement('div');
    meta.className = 'meta';
    meta.textContent = describe(report);
    body.append(summary, meta);
    const state = document.createElement('div');
    state.className = 'state';
    if (report.issue) {
      const link = document.createElement('a');
      link.href = report.issue;
      link.target = '_blank';
      link.rel = 'noopener';
      link.textContent = '#' + report.issue.split('/').pop();
      state.append(link);
    }
    row.append(kind, body, state);
    row.addEventListener('click', e => {
      if (e.target.tagName !== 'A') {
        openReport(report.id);
      }
    });
    list.append(row);
  }
}

function renderCount() {
  const badge = document.querySelector('#feedback-count');
  const waiting = feedback.rows.filter(r => r.status === 'New').length;
  badge.textContent = waiting;
  badge.hidden = !waiting;
}

async function loadFeedback() {
  const res = await fetch('/api/admin/feedback');
  if (!res.ok) {
    document.querySelector('#feedback-rows').textContent = 'Could not load the reports.';
    return;
  }
  const state = await res.json();
  feedback.rows = state.reports;
  feedback.canFile = state.canFile;
  feedback.repo = state.repo;
  renderCount();
  renderReports();
}

async function openReport(id) {
  feedback.open = id;
  renderReports();
  const card = document.querySelector('#feedback-detail');
  card.hidden = false;
  card.replaceChildren();
  card.className = 'card report-detail';
  const res = await fetch('/api/admin/feedback/' + encodeURIComponent(id));
  if (!res.ok) {
    card.textContent = 'Could not open that report.';
    return;
  }
  const report = await res.json();
  const heading = document.createElement('h3');
  heading.textContent = report.summary;
  const said = document.createElement('div');
  said.className = 'said';
  said.textContent = report.details || 'No details given.';
  const facts = document.createElement('dl');
  const rows = [
    ['Reporter', `${report.email} (${report.role})`],
    ['App', report.appName],
    ['Page', report.page],
    ['Address', report.url],
    ['Browser', report.browser],
    ['Viewport', [report.viewport, report.screen].filter(Boolean).join(' of ')],
    ['Language', [report.language, report.timezone].filter(Boolean).join(' · ')],
    ['Errors', (report.errors || []).join('\n')],
  ];
  for (const [name, value] of rows) {
    if (!value) {
      continue;
    }
    const dt = document.createElement('dt');
    dt.textContent = name;
    const dd = document.createElement('dd');
    dd.textContent = value;
    facts.append(dt, dd);
  }
  card.append(heading, said, facts);

  if (report.status !== 'New') {
    const done = document.createElement('div');
    done.className = 'hint';
    done.textContent = report.status === 'Filed'
      ? `Filed by ${report.handledBy}.`
      : `Dismissed by ${report.handledBy}.`;
    card.append(done);
    if (report.issue) {
      const link = document.createElement('a');
      link.className = 'button';
      link.href = report.issue;
      link.target = '_blank';
      link.rel = 'noopener';
      link.textContent = 'Open the issue';
      const actions = document.createElement('div');
      actions.className = 'card-actions';
      actions.append(link);
      card.append(actions);
    }
    return;
  }

  const hint = document.createElement('div');
  hint.className = 'hint';
  hint.textContent = feedback.canFile
    ? `This is what ${feedback.repo} will get, with the reporter's address already taken out. Edit it into an issue worth keeping, then file it.`
    : 'Filing on GitHub is not set up on this server, so this report can only be dismissed.';
  const title = field('input', 'Title', report.draft.title, {type: 'text', maxLength: 200});
  const body = field('textarea', 'Body', report.draft.body);
  const labels = field('input', 'Labels', (report.draft.labels || []).join(', '), {type: 'text'});
  const actions = document.createElement('div');
  actions.className = 'report-actions';
  const status = document.createElement('span');
  status.className = 'save-status';
  const fileButton = document.createElement('button');
  fileButton.className = 'button';
  fileButton.type = 'button';
  fileButton.textContent = 'File on GitHub';
  fileButton.disabled = !feedback.canFile;
  const dismissButton = document.createElement('button');
  dismissButton.className = 'link-button';
  dismissButton.type = 'button';
  dismissButton.textContent = 'Dismiss';

  fileButton.addEventListener('click', async () => {
    fileButton.disabled = true;
    status.classList.remove('error');
    status.textContent = 'Filing…';
    const res = await fetch(`/api/admin/feedback/${encodeURIComponent(id)}/file`, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        title: title.input.value,
        body: body.input.value,
        labels: labels.input.value.split(',').map(l => l.trim()).filter(Boolean),
      }),
    });
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = await res.text();
      fileButton.disabled = false;
      return;
    }
    await loadFeedback();
    openReport(id);
  });

  dismissButton.addEventListener('click', async () => {
    status.classList.remove('error');
    status.textContent = 'Dismissing…';
    const res = await fetch(`/api/admin/feedback/${encodeURIComponent(id)}/dismiss`, {method: 'POST'});
    if (!res.ok) {
      status.classList.add('error');
      status.textContent = await res.text();
      return;
    }
    await loadFeedback();
    openReport(id);
  });

  actions.append(fileButton, dismissButton, status);
  card.append(hint, title.wrap, body.wrap, labels.wrap, actions);
}

function initFeedback() {
  for (const chip of document.querySelectorAll('.feedback-filters .chip')) {
    chip.addEventListener('click', () => {
      for (const other of document.querySelectorAll('.feedback-filters .chip')) {
        other.classList.toggle('active', other === chip);
      }
      feedback.filter = chip.dataset.status;
      renderReports();
    });
  }
}

// The word of a new report links straight to it.
async function followLink() {
  const params = new URLSearchParams(location.search);
  if (params.get('panel') !== 'feedback') {
    return;
  }
  showPanel('feedback');
  const id = params.get('report');
  if (id) {
    await openReport(id);
    document.querySelector('#feedback-detail').scrollIntoView({behavior: 'smooth', block: 'start'});
  }
}

async function load() {
  const res = await fetch('/api/admin/state');
  if (!res.ok) {
    document.body.innerHTML = '<p style="padding:28px;font-family:sans-serif">' +
      (res.status === 403 ? 'Admin access required.' : 'Failed to load admin state.') + '</p>';
    return;
  }
  const state = await res.json();
  document.querySelector('#me').textContent = state.email;
  const notice = document.querySelector('#store-notice');
  notice.replaceChildren();
  if (!state.hasStore) {
    const div = document.createElement('div');
    div.className = 'notice';
    div.textContent = 'Running in sample mode: image uploads are disabled because there is no media bucket configured.';
    notice.append(div);
  }
  listEditor({
    rows: document.querySelector('#admins-rows'),
    status: document.querySelector('#admins-status'),
    input: document.querySelector('#add-admin-email'),
    button: document.querySelector('#add-admin-button'),
    url: '/api/admin/admins',
    body: admins => ({admins}),
    empty: 'Nobody yet.',
    initial: state.admins,
  });
  loadCategories();
  // The reports carry who reported them and what page they were on, so the
  // queue is the super admins' alone; for anyone else the tab is not there.
  if (!state.isSuperAdmin) {
    document.querySelector('.tab[data-panel="feedback"]').remove();
    document.querySelector('#panel-feedback').remove();
    return;
  }
  await loadFeedback();
  await followLink();
}

initTabs();
initFeedback();
load();
