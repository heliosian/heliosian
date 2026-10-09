import {state, model, emailOf, isStudent, familyOf, kidsOf, adultsOf, viewerId, q, rowsOf, write} from '../state.js';
import {firstName, lastName, hue, csvField} from '../dom.js';
import {dataGrid} from '/datagrid.js';
import {el, svg, link} from '/elements.js';
import {familyLink, familySearchText} from '../families.js';
import {personLink} from '../people.js';
import {tagFacetOptions} from '../tags.js';
import {saveTagRelations} from '../storage.js';
import {anyFiltersActive, matchesFilters, familyMatchesFilters, roleChips, gradeOptions, classroomOptions, tagRelationOptionsFor} from '../filters.js';
import {facetDropdown, clampFilterPanel} from '/rules.js';
import {render} from '/router.js';
import {openLayer} from '/modal.js';

function joinFamilyNames(people) {
  if (!people.length) {
    return '';
  }
  if (people.length === 1) {
    return people[0].name_show;
  }
  const surname = lastName(people[0].name_show);
  const shared = surname && people.every(p => lastName(p.name_show) === surname);
  const names = people.map(p => shared ? firstName(p.name_show) : p.name_show);
  const line = names.length === 2 ? names.join(' & ') : `${names.slice(0, -1).join(', ')} & ${names[names.length - 1]}`;
  return shared ? `${line} ${surname}` : line;
}

function joinFirstNames(people) {
  if (!people.length) {
    return '';
  }
  const names = people.map(p => firstName(p.name_show));
  return names.length === 1 ? names[0] : `${names.slice(0, -1).join(', ')} & ${names[names.length - 1]}`;
}

const GREETING_WHOLE_FAMILY = 'Ali, Bo, Pat & Quinn Ender';
const GREETING_KIDS = 'Ali & Bo Ender';
const GREETING_ADULTS = 'Pat & Quinn Ender';
const GREETING_FULL_NAME = 'Pat Ender';
const GREETING_FIRST_NAME = 'Pat';
const GREETING_DEFAULT = 'Family of Ali & Bo Ender';

function greetingSurname({kids, adults, person}) {
  if (person) {
    return lastName(person.name_show);
  }
  const primary = adults[0] || kids[0];
  return primary ? lastName(primary.name_show) : '';
}

function splitLastFirst(people) {
  if (!people.length) {
    return {rest: '', last: ''};
  }
  const names = people.map(p => firstName(p.name_show));
  return {rest: names.slice(0, -1).join(', '), last: names[names.length - 1]};
}

function replaceRestToken(text, token, value) {
  if (value) {
    return text.replace(new RegExp(`\\b${token}\\b`, 'g'), value);
  }
  return text
    .replace(new RegExp(`\\b${token}\\b,\\s*`, 'g'), '')
    .replace(new RegExp(`\\b${token}\\b\\s*&\\s*`, 'g'), '')
    .replace(new RegExp(`\\b${token}\\b`, 'g'), '');
}

function substituteNameTokens(text, {kids, adults, person}) {
  if (!person) {
    const kidsSplit = splitLastFirst(kids);
    const adultsSplit = splitLastFirst(adults);
    text = replaceRestToken(text, 'Ali', kidsSplit.rest);
    text = text.replace(/\bBo\b/g, kidsSplit.last);
    text = replaceRestToken(text, 'Pat', adultsSplit.rest);
    text = text.replace(/\bQuinn\b/g, adultsSplit.last);
  }
  const surname = greetingSurname({kids, adults, person});
  return text.replace(/\bEnder(s?)\b/g, `${surname}$1`);
}

function buildGreeting(format, {kids = [], adults = [], person, siblings = []} = {}) {
  let text = format;
  if (GREETING_WHOLE_FAMILY && text.includes(GREETING_WHOLE_FAMILY)) {
    text = text.replaceAll(GREETING_WHOLE_FAMILY, joinFamilyNames([...kids, ...adults]));
  }
  if (GREETING_KIDS && text.includes(GREETING_KIDS)) {
    text = text.replaceAll(GREETING_KIDS, joinFamilyNames(kids.length ? kids : adults));
  }
  if (GREETING_ADULTS && text.includes(GREETING_ADULTS)) {
    text = text.replaceAll(GREETING_ADULTS, joinFamilyNames(adults));
  }
  if (person) {
    const group = [person, ...siblings];
    if (GREETING_FULL_NAME && text.includes(GREETING_FULL_NAME)) {
      text = text.replaceAll(GREETING_FULL_NAME, joinFamilyNames(group));
    }
    if (GREETING_FIRST_NAME && text.includes(GREETING_FIRST_NAME)) {
      text = text.replaceAll(GREETING_FIRST_NAME, joinFirstNames(group));
    }
  }
  return substituteNameTokens(text, {kids, adults, person, siblings});
}

function greetingFormatsFor(inviteBy, supportsGroups) {
  const wantGrouped = inviteBy === 'group' && supportsGroups;
  return inviteGreetings.filter(g => wantGrouped ? g.grouped : g.individual);
}

function buildInviteParams(addressee, addresseeContact, greeting, members) {
  const params = {
    greeting,
    primary_email: addresseeContact,
    primary_first_name: firstName(addressee.name_show),
    primary_last_name: lastName(addressee.name_show),
    primary_phone: addressee.phone || '',
  };
  members.forEach((m, i) => {
    params[`member_${i + 1}`] = m.contact ? `${m.name} <${m.contact}>` : m.name;
  });
  return {
    greeting,
    people: [{name: addressee.name_show, contact: addresseeContact}, ...members],
    params,
  };
}

function kidContact(kid) {
  return state.gvKidEmail ? emailOf(kid) : '';
}

function invitedKids(family) {
  const kids = kidsOf(family);
  if (state.gvSiblings) {
    return kids;
  }
  if (!anyFiltersActive() && !state.q) {
    return [];
  }
  return kids.filter(k => matchesFilters(k) &&
    (!state.q || k.name_show.toLowerCase().includes(state.q) || emailOf(k).toLowerCase().includes(state.q)));
}

function familyInviteParams(family) {
  const adults = adultsOf(family).filter(emailOf);
  if (!adults.length) {
    return null;
  }
  const kids = invitedKids(family);
  const [primary, ...otherAdults] = adults;
  const format = inviteGreetings.find(g => g.id === state.gvGreeting) || greetingFormatsFor('group', true)[0];
  const greeting = format ? buildGreeting(format.format, {kids, adults}) : '';
  const members = otherAdults.map(a => ({name: a.name_show, contact: emailOf(a)}))
    .concat(kids.map(k => ({name: k.name_show, contact: kidContact(k)})));
  const anchor = kids[0] || primary;
  return {
    ...buildInviteParams(primary, emailOf(primary), greeting, members),
    linkHref: familyLink(family),
    sortKey: lastName(anchor.name_show) + ' ' + firstName(anchor.name_show),
  };
}

function individualCandidates(p) {
  if (!isStudent(p)) {
    return emailOf(p) ? [{contact: emailOf(p), person: p}] : [];
  }
  const family = familyOf(p);
  let kids = [p];
  if (state.gvSiblings && family) {
    kids = invitedKids(family);
  }
  const parents = family ? adultsOf(family).filter(emailOf) : [];
  if (!parents.length) {
    return kids.map(kid => ({contact: '', person: kid}));
  }
  const candidates = [];
  for (const parent of parents) {
    for (const kid of kids) {
      candidates.push({contact: emailOf(parent), person: kid});
    }
  }
  return candidates;
}

function buildMergedEntry(people, contact) {
  const addressee = people[0];
  const format = inviteGreetings.find(g => g.id === state.gvGreeting) || greetingFormatsFor('individual', false)[0];
  const greeting = format ? buildGreeting(format.format, {person: addressee, siblings: people.slice(1)}) : '';
  return {
    ...buildInviteParams(addressee, contact, greeting, []),
    linkHref: personLink(addressee),
    sortKey: lastName(addressee.name_show) + ' ' + firstName(addressee.name_show),
  };
}

function mergeCandidates(candidates) {
  const byContact = new Map();
  const rows = [];
  for (const c of candidates) {
    if (!c.contact) {
      rows.push(buildMergedEntry([c.person], ''));
      continue;
    }
    if (!byContact.has(c.contact)) {
      byContact.set(c.contact, []);
    }
    const people = byContact.get(c.contact);
    if (!people.some(x => x.id === c.person.id)) {
      people.push(c.person);
    }
  }
  for (const [contact, people] of byContact) {
    rows.push(buildMergedEntry(people, contact));
  }
  return rows;
}

function invitesEntries() {
  const rows = [];
  if (state.gvInviteBy === 'individual') {
    const matches = p => matchesFilters(p) && (p.name_show.toLowerCase().includes(state.q) || emailOf(p).toLowerCase().includes(state.q));
    const candidates = [];
    // Adults go first so a merged row is addressed to the adult, not their kid.
    for (const p of model.people) {
      if (!isStudent(p) && matches(p)) {
        candidates.push(...individualCandidates(p));
      }
    }
    const mergedFamilies = new Set();
    for (const p of model.people) {
      if (!isStudent(p) || !matches(p)) {
        continue;
      }
      if (state.gvSiblings) {
        const family = familyOf(p);
        if (family) {
          if (mergedFamilies.has(family.id)) {
            continue;
          }
          mergedFamilies.add(family.id);
        }
      }
      candidates.push(...individualCandidates(p));
    }
    rows.push(...mergeCandidates(candidates));
  } else {
    for (const family of model.families) {
      if (!familyMatchesFilters(family) || !familySearchText(family).includes(state.q)) {
        continue;
      }
      const entry = familyInviteParams(family);
      if (entry) {
        rows.push(entry);
      }
    }
  }
  rows.sort((a, b) => a.sortKey.localeCompare(b.sortKey));
  return rows;
}

function fillTemplate(template, params) {
  return (template || '').replace(/\{\{\s*([\w.]+)\s*\}\}/g, (_, key) => params[key] ?? '');
}

function templateRepeatsPerMember(system) {
  return system.columns.some(c => /\{\{\s*member_(name|contact)\s*\}\}/.test(c.template));
}

function isGreetingTemplate(template) {
  return /\{\{\s*greeting\s*\}\}/.test(template);
}

function templateUsesGreeting(system) {
  return system.columns.some(c => isGreetingTemplate(c.template));
}

function applyInviteTemplate(system, entries) {
  const perMember = templateRepeatsPerMember(system);
  const rows = [];
  for (const entry of entries) {
    if (!perMember) {
      rows.push({entry, cells: system.columns.map(c => fillTemplate(c.template, entry.params))});
      continue;
    }
    for (const person of entry.people) {
      const params = {...entry.params, member_name: person.name, member_contact: person.contact};
      rows.push({entry, cells: system.columns.map(c => fillTemplate(c.template, params))});
    }
  }
  return rows;
}

let inviteSystems = null;
let inviteGreetings = [];
let inviteLoadError = '';

const yes = cell => cell === 'Yes';

async function loadInviteSystems() {
  if (inviteSystems) {
    return inviteSystems;
  }
  inviteLoadError = '';
  inviteSystems = [];
  inviteGreetings = [];
  try {
    const [services, templates, greetings] = await Promise.all([
      q('(from INVITE_SERVICE (order name asc))'),
      q('(from INVITE_TEMPLATE (order order asc))'),
      q('(from GREETING (order name asc))'),
    ]);
    const columns = rowsOf(templates, 'INVITE_TEMPLATE');
    inviteSystems = rowsOf(services, 'INVITE_SERVICE').map(s => ({
      id: s.id,
      name: s.name,
      description: s.description || '',
      headerRow: yes(s.header_row),
      supportsGroups: yes(s.grouped),
      columns: columns.filter(c => c.service === s.id).map(c => ({name: c.column || '', template: c.template || ''})),
    }));
    inviteGreetings = rowsOf(greetings, 'GREETING').map(g => ({
      id: g.id,
      name: g.name,
      format: g.format || '',
      grouped: yes(g.grouped),
      individual: yes(g.individual),
      mine: Boolean(g.added_by) && g.added_by === viewerId(),
    }));
  } catch (err) {
    inviteLoadError = err.message;
  }
  return inviteSystems;
}

function defaultGreeting() {
  return inviteGreetings.find(g => !g.mine && g.format === GREETING_DEFAULT) || inviteGreetings[0];
}

export function invitesPage() {
  const page = document.createDocumentFragment();

  const pageHeader = el('div', 'page-header container page-header-list');
  const titleWrap = el('div');
  titleWrap.append(el('h1', 'page-title', 'Invites'));
  titleWrap.append(el('div', 'page-subtitle', 'Export a formatted CSV of families, grouped and greeted the way you choose, for invitations.'));
  pageHeader.append(titleWrap);
  page.append(pageHeader);

  const settings = el('div', 'gv-settings container');
  const loading = el('div', 'gv-setting-group', 'Loading invite templates…');
  settings.append(loading);
  page.append(settings);

  const content = el('div', 'content container');
  page.append(content);

  loadInviteSystems().then(systems => {
    if (!systems.length) {
      settings.replaceChildren();
      content.replaceChildren();
      settings.append(el('div', 'gv-setting-group', inviteLoadError
        ? `Couldn't load invite templates: ${inviteLoadError}`
        : 'No invite services are set up yet - add one as an INVITE_SERVICE row with its INVITE_TEMPLATE columns.'));
      return;
    }
    if (!systems.some(s => s.id === state.gvSystem)) {
      state.gvSystem = systems[0].id;
    }
    if (!inviteGreetings.some(g => g.id === state.gvGreeting)) {
      state.gvGreeting = defaultGreeting().id;
    }
    renderInvites(systems, settings, content);
  });
  return page;
}

function renderInvites(systems, settings, content) {
  settings.replaceChildren();
  content.replaceChildren();
  const view = {systems, grid: el('div'), download: el('a', 'filter-button email-download')};
  const again = () => renderInvites(systems, settings, content);
  const paint = () => renderInviteGrid(view, again);
  renderInviteSettings(systems, settings, again, paint);
  const {header, input} = invitesControls(view, again, paint);
  content.append(header, view.grid);
  paint();
  input.focus();
}

function invitesControls(view, again, paint) {
  const header = el('div', 'content-header content-header-solo');
  const controls = el('div', 'controls');
  controls.append(roleChips(paint));
  const search = el('div', 'search');
  search.append(svg('search'));
  const input = el('input');
  input.placeholder = 'Search';
  input.value = state.q;
  input.addEventListener('input', () => {
    state.q = input.value.trim().toLowerCase();
    paint();
  });
  search.append(input);
  controls.append(
    facetDropdown('Grade', null, gradeOptions(), state.filterGrades, paint),
    facetDropdown('Classroom', null, classroomOptions(), state.filterClassrooms, paint),
  );
  if (tagFacetOptions().length) {
    controls.append(facetDropdown('Tags', null, tagFacetOptions(), state.filterTags, again));
  }
  const relationOptions = state.filterTags.size === 1 ? tagRelationOptionsFor([...state.filterTags][0]) : [];
  if (relationOptions.length) {
    const [activeTag] = state.filterTags;
    controls.append(facetDropdown('Include', null, relationOptions, state.filterTagRelations, () => {
      saveTagRelations(activeTag, state.filterTagRelations);
      paint();
    }));
  }
  const {download} = view;
  download.title = 'Download what the list currently shows';
  download.append(svg('download'), el('span', '', 'CSV'));
  controls.append(search, download);
  header.append(controls);
  return {header, input};
}

function renderInviteGrid(view, again) {
  const {systems, grid, download} = view;
  grid.replaceChildren();
  const system = systems.find(s => s.id === state.gvSystem) || systems[0];
  const rows = applyInviteTemplate(system, invitesEntries());
  const header = system.columns.map(c => c.name);
  const csvLines = rows.map(r => r.cells.map(csvField).join(','));
  if (system.headerRow) {
    csvLines.unshift(header.map(csvField).join(','));
  }
  download.href = 'data:text/csv;charset=utf-8,' + encodeURIComponent(csvLines.join('\n'));
  download.download = system.name.toLowerCase().replaceAll(' ', '-') + '.csv';
  if (!rows.length) {
    grid.append(el('div', 'empty', 'No matches.'));
    return;
  }
  const columns = system.columns.map((column, i) => ({
    label: column.name,
    get: r => r.cells[i],
    show: i === 0 ? inviteLink : undefined,
    head: isGreetingTemplate(column.template) ? greetingHeadSelect(system, again) : undefined,
  }));
  grid.append(dataGrid({columns, rows}).wrap);
}

function inviteLink(r) {
  return link(r.entry.linkHref, '', r.cells[0]);
}

function appendGreetingOptions(parent, list) {
  for (const f of list) {
    const option = el('option', '', f.name);
    option.value = f.id;
    parent.append(option);
  }
}

function greetingHeadSelect(system, again) {
  const headSelect = el('select', 'gv-select gv-th-select');
  const formats = greetingFormatsFor(state.gvInviteBy, system.supportsGroups);
  const mine = formats.filter(f => f.mine);
  if (mine.length) {
    const mineGroup = el('optgroup');
    mineGroup.label = 'Yours';
    appendGreetingOptions(mineGroup, mine);
    const everyoneGroup = el('optgroup');
    everyoneGroup.label = 'Everyone’s';
    appendGreetingOptions(everyoneGroup, formats.filter(f => !mine.includes(f)));
    headSelect.append(mineGroup, everyoneGroup);
  } else {
    appendGreetingOptions(headSelect, formats);
  }
  const divider = el('option', '', '──────────');
  divider.disabled = true;
  const editOption = el('option', '', '(Edit Greetings)');
  editOption.value = '__new__';
  headSelect.append(divider, editOption);
  headSelect.value = state.gvGreeting;
  headSelect.addEventListener('change', () => {
    if (headSelect.value === '__new__') {
      headSelect.value = state.gvGreeting;
      openGreetingDialog(render);
      return;
    }
    state.gvGreeting = headSelect.value;
    again();
  });
  return headSelect;
}

const serviceLogos = {
  Greenvelope: '/services/greenvelope.png',
  Evite: '/services/evite.png',
  'Paperless Post': '/services/paperless-post.png',
  Punchbowl: '/services/punchbowl.png',
  Partiful: '/services/partiful.jpg',
};

function serviceIcon(name) {
  const url = serviceLogos[name];
  if (url) {
    const img = el('img', 'gv-service-logo');
    img.src = url;
    img.alt = '';
    return img;
  }
  const div = el('div', 'gv-service-logo gv-service-fallback', (name[0] || '?').toUpperCase());
  div.style.background = `hsl(${hue(name)} 45% 55%)`;
  return div;
}

function gvServiceSelect(seg, systems, onChange) {
  const wrap = el('div', 'filter-wrap gv-service-wrap');
  const button = el('button', 'filter-button gv-service-button');
  button.type = 'button';
  const current = systems.find(s => s.id === state.gvSystem) || systems[0];
  button.append(serviceIcon(current.name), el('span', '', current.name), svg('chevron-down'));
  const panel = el('div', 'filter-panel gv-service-panel');
  panel.hidden = true;
  button.addEventListener('click', () => {
    panel.hidden = !panel.hidden;
    button.classList.toggle('open', !panel.hidden);
    if (!panel.hidden) {
      panel.style.width = `${wrap.offsetWidth}px`;
      clampFilterPanel(wrap, panel);
    }
  });
  for (const s of systems) {
    const option = el('div', 'filter-option gv-service-option');
    option.append(serviceIcon(s.name), el('span', '', s.name));
    option.addEventListener('click', () => onChange(s.id));
    panel.append(option);
  }
  wrap.append(button, panel);
  seg.append(wrap);
}

function defaultGreetingFormat() {
  return state.gvInviteBy === 'group' ? `The ${GREETING_WHOLE_FAMILY} Family` : `Dear ${GREETING_FIRST_NAME}`;
}

const GREETING_PREVIEW_FAMILY = {
  kids: [{name_show: 'Nora Rivera'}, {name_show: 'Theo Rivera'}],
  adults: [{name_show: 'Sam Rivera'}, {name_show: 'Jamie Rivera'}],
};

function openGreetingDialog(onSaved) {
  const overlay = el('div', 'greeting-dialog-overlay');
  const panel = el('div', 'greeting-dialog-panel');
  const close = openLayer(() => {
    overlay.remove();
    document.removeEventListener('keydown', onKey);
  });
  function onKey(e) {
    if (e.key === 'Escape') {
      close();
    }
  }
  overlay.addEventListener('click', e => {
    if (e.target === overlay) {
      close();
    }
  });
  document.addEventListener('keydown', onKey);
  const dialog = greetingDialogState(close, onSaved);
  panel.append(greetingDialogHeader(close), greetingForm(dialog));
  overlay.append(panel);
  document.body.append(overlay);
  dialog.formatInput.focus();
  dialog.formatInput.select();
}

function greetingDialogState(close, onSaved) {
  const formatInput = el('input');
  formatInput.maxLength = 200;
  formatInput.required = true;
  const error = el('div', 'gv-new-greeting-error');
  error.hidden = true;
  const previewNote = el('div', 'gv-new-greeting-preview-note',
    'No phrase matched, so this will show exactly as typed for every family - fine for a fixed line, not if you meant to personalize it.');
  previewNote.hidden = true;
  return {
    close,
    onSaved,
    grouped: state.gvInviteBy === 'group',
    editing: '',
    cards: [],
    formatInput,
    previewValue: el('span', 'gv-new-greeting-preview-value'),
    previewNote,
    editorSection: el('div', 'gv-greeting-editor'),
    error,
  };
}

function greetingDialogHeader(close) {
  const header = el('div', 'greeting-dialog-header');
  const headerIcon = el('div', 'greeting-dialog-icon');
  headerIcon.append(svg('sparkle'));
  const headerText = el('div', 'greeting-dialog-header-text');
  headerText.append(
    el('div', 'greeting-dialog-title', 'Edit Greeting'),
    el('div', 'greeting-dialog-subtitle', 'Create a sample greeting for this family.'),
  );
  const headerClose = el('button', 'greeting-dialog-close');
  headerClose.type = 'button';
  headerClose.setAttribute('aria-label', 'Close');
  headerClose.append(svg('close'));
  headerClose.addEventListener('click', close);
  header.append(headerIcon, headerText, headerClose);
  return header;
}

function greetingForm(dialog) {
  const {formatInput, editorSection} = dialog;
  const form = el('form', 'gv-new-greeting-form');
  formatInput.addEventListener('input', () => updateGreetingPreview(dialog));
  const mine = inviteGreetings.filter(g => g.mine);
  if (mine.length) {
    form.append(greetingList(dialog, mine));
    editorSection.append(el('div', 'gv-greeting-divider'));
  }
  formatInput.value = defaultGreetingFormat();
  const formatLabel = el('label', 'gv-new-greeting-field');
  formatLabel.append(el('span', '', 'Greeting'), formatInput);
  const preview = el('div', 'gv-new-greeting-preview');
  preview.append(el('div', 'gv-new-greeting-preview-label', 'Preview'), dialog.previewValue);
  editorSection.append(formatLabel, preview, dialog.previewNote);
  updateGreetingPreview(dialog);
  const hint = el('div', 'gv-new-greeting-hint',
    'Use the family’s actual names (e.g., Pat, Quinn, Ali, Bo) in your own words.');
  const actions = el('div', 'gv-new-greeting-actions');
  const cancel = el('button', 'gv-new-greeting-cancel', 'Cancel');
  cancel.type = 'button';
  cancel.addEventListener('click', dialog.close);
  const save = el('button', 'filter-done gv-new-greeting-save', 'Save');
  save.type = 'submit';
  actions.append(cancel, save);
  editorSection.append(hint, dialog.error, actions);
  editorSection.hidden = mine.length > 0;
  form.append(editorSection);
  form.addEventListener('submit', e => {
    e.preventDefault();
    saveGreeting(dialog, save);
  });
  return form;
}

function updateGreetingPreview(dialog) {
  const format = dialog.formatInput.value;
  const rendered = dialog.grouped
    ? buildGreeting(format, GREETING_PREVIEW_FAMILY)
    : buildGreeting(format, {person: GREETING_PREVIEW_FAMILY.adults[0]});
  dialog.previewValue.textContent = rendered || '(empty)';
  dialog.previewNote.hidden = rendered !== format;
}

function loadGreeting(dialog, g) {
  dialog.editing = g ? g.id : '';
  dialog.formatInput.value = g ? g.format : defaultGreetingFormat();
  dialog.cards.forEach(({card, greeting}) => card.classList.toggle('active', greeting === g));
  dialog.editorSection.hidden = false;
  updateGreetingPreview(dialog);
  dialog.formatInput.focus();
  dialog.formatInput.select();
}

function greetingList(dialog, mine) {
  const listWrap = el('div', 'gv-greeting-list');
  listWrap.append(el('div', 'gv-greeting-list-title', 'Your greetings'));
  for (const g of mine) {
    listWrap.append(greetingCard(dialog, g));
  }
  const addButton = el('button', 'gv-greeting-add');
  addButton.type = 'button';
  const addIcon = el('span', 'gv-greeting-add-icon');
  addIcon.append(svg('plus'));
  addButton.append(addIcon, el('span', '', 'Add another greeting'));
  addButton.addEventListener('click', () => loadGreeting(dialog, null));
  listWrap.append(addButton);
  return listWrap;
}

function greetingCard(dialog, g) {
  const card = el('div', 'gv-greeting-card');
  card.append(el('span', 'gv-greeting-card-text', g.format));
  const cardActions = el('div', 'gv-greeting-card-actions');
  const editBtn = el('button', 'gv-greeting-card-btn');
  editBtn.type = 'button';
  editBtn.title = 'Edit';
  editBtn.append(svg('edit'));
  editBtn.addEventListener('click', () => loadGreeting(dialog, g));
  const deleteBtn = el('button', 'gv-greeting-card-btn gv-greeting-card-btn-delete');
  deleteBtn.type = 'button';
  deleteBtn.title = 'Delete';
  deleteBtn.append(svg('trash'));
  deleteBtn.addEventListener('click', () => deleteGreeting(dialog, g, deleteBtn));
  cardActions.append(editBtn, deleteBtn);
  card.append(cardActions);
  dialog.cards.push({card, greeting: g});
  return card;
}

async function deleteGreeting(dialog, g, deleteBtn) {
  const {error} = dialog;
  if (!confirm(`Delete "${g.format}"? This can't be undone.`)) {
    return;
  }
  error.hidden = true;
  deleteBtn.disabled = true;
  try {
    await write([{delete: g.id}]);
    inviteSystems = null;
    dialog.close();
    dialog.onSaved();
  } catch (err) {
    error.hidden = false;
    error.textContent = 'Couldn’t delete that greeting - try again.';
    deleteBtn.disabled = false;
  }
}

async function saveGreeting(dialog, save) {
  const {error} = dialog;
  const format = dialog.formatInput.value.trim();
  if (!format) {
    return;
  }
  error.hidden = true;
  save.disabled = true;
  try {
    const cells = {name: format, format, grouped: state.gvInviteBy === 'group', individual: state.gvInviteBy !== 'group'};
    if (dialog.editing) {
      await write([{set: dialog.editing, cells}]);
      state.gvGreeting = dialog.editing;
    } else {
      state.gvGreeting = (await write([{insert: 'GREETING', row: {...cells, added_by: viewerId()}}])).result[0];
    }
    inviteSystems = null;
    dialog.close();
    dialog.onSaved();
  } catch (err) {
    error.hidden = false;
    error.textContent = 'Couldn’t save that greeting - try again.';
    save.disabled = false;
  }
}

function gvSegment(row, label, infoText) {
  const seg = el('div', 'gv-settings-segment');
  const labelRow = el('div', 'gv-setting-label');
  labelRow.append(el('span', '', label));
  if (infoText) {
    const info = el('button', 'gv-info-btn');
    info.type = 'button';
    info.title = infoText;
    info.setAttribute('aria-label', label + ': ' + infoText);
    info.append(svg('info'));
    labelRow.append(info);
  }
  seg.append(labelRow);
  row.append(seg);
  return seg;
}

function gvSwitch(seg, checked, onChange) {
  const toggle = el('input', 'filter-switch');
  toggle.type = 'checkbox';
  toggle.checked = checked;
  toggle.addEventListener('change', () => onChange(toggle.checked));
  seg.append(toggle);
  return toggle;
}

function renderInviteSettings(systems, settings, onSystemChange, onSettingChange) {
  const system = systems.find(s => s.id === state.gvSystem) || systems[0];
  if (!system.supportsGroups) {
    state.gvInviteBy = 'individual';
  }
  if (templateUsesGreeting(system)) {
    const formats = greetingFormatsFor(state.gvInviteBy, system.supportsGroups);
    if (formats.length && !formats.some(f => f.id === state.gvGreeting)) {
      state.gvGreeting = formats[0].id;
    }
  }

  const bar = el('div', 'gv-settings-bar');
  const row = el('div', 'gv-settings-row');
  bar.append(row);

  const exportSeg = gvSegment(row, 'Export for');
  gvServiceSelect(exportSeg, systems, key => {
    const next = systems.find(s => s.id === key);
    state.gvSystem = key;
    state.gvInviteBy = next && next.supportsGroups ? 'group' : 'individual';
    onSystemChange();
  });

  if (system.supportsGroups) {
    row.append(el('div', 'gv-settings-divider'));
    const groupSeg = gvSegment(row, 'Group by family',
      'On: one invite per family, addressed to everyone in the household together. Off: one invite per person.');
    gvSwitch(groupSeg, state.gvInviteBy === 'group', checked => {
      state.gvInviteBy = checked ? 'group' : 'individual';
      onSystemChange();
    });
  }

  row.append(el('div', 'gv-settings-divider'));
  const siblingsSeg = gvSegment(row, 'Include siblings', 'Will include the children or siblings of invitees.');
  gvSwitch(siblingsSeg, state.gvSiblings, checked => {
    state.gvSiblings = checked;
    onSettingChange();
  });

  if (state.gvInviteBy === 'group') {
    row.append(el('div', 'gv-settings-divider'));
    const emailSeg = gvSegment(row, 'Send to Kid Emails', 'Will include the student emails of the invitees.');
    gvSwitch(emailSeg, state.gvKidEmail, checked => {
      state.gvKidEmail = checked;
      onSettingChange();
    });
  }

  if (system.description) {
    bar.append(el('div', 'gv-fyi gv-settings-summary', system.description));
  }
  settings.append(bar);
}
