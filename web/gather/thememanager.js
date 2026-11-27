import {el, svg, button, toast} from '/elements.js';
import {openModal} from '/modal.js';
import {field, text} from '/form.js';
import {queries, rowsOf, write} from '/rail.js';
import {loadThemes, refreshThemes} from '/themes.js';
import {miniEnvelope} from '/envelope.js';
import {me, removeTheme} from './state.js';
import {themeUpload, deleteThemeWords} from './editor.js';

async function allThemes() {
  const a = await queries({
    themes: '(from THEME (order name asc) (columns key name blurb added_by))',
    owners: '(from PERSON (where (in id (select THEME.added_by (not (blank added_by))))) (columns name_show))',
  });
  const rows = rowsOf(a.themes, 'THEME');
  await loadThemes(rows.map(r => r.id));
  const owners = new Map(rowsOf(a.owners, 'PERSON').map(p => [p.id, p.name_show]));
  return rows.map(r => ({...r, owner: r.added_by ? owners.get(r.added_by) || 'Someone' : ''}));
}

function renamePopup(t, done) {
  const name = text(t.name, {required: true});
  const blurb = text(t.blurb || '', {placeholder: 'A line about it for the Theme picker'});
  openModal(`Rename ${t.name}`, [field('Name', name, '', true), field('Blurb', blurb, 'The line under its name in the Theme picker.')], {
    submit: () => write([{set: t.id, cells: {name: name.value.trim(), blurb: blurb.value.trim()}}]),
    afterSave: done,
  });
}

function themeRow(t, redraw) {
  const row = el('li', 'theme-manage-row');
  row.dataset.theme = t.id;
  const picture = el('div', 'theme-manage-picture');
  picture.append(miniEnvelope({theme: t.id}, t.name));
  const words = el('div', 'theme-manage-words');
  words.append(el('span', 'theme-manage-name', t.name), el('span', 'theme-manage-blurb', t.blurb || ''));
  const badge = el('span', 'theme-manage-badge' + (t.owner ? ' is-private' : ' is-public'));
  badge.append(svg(t.owner ? 'lock' : 'groups'), el('span', '', t.owner ? 'Private' : 'Public'));
  const owner = el('div', 'theme-manage-owner');
  owner.append(badge, el('span', '', t.owner ? `Only ${t.owner} can pick it` : 'Every host can pick it'));
  words.append(owner);
  const actions = el('div', 'theme-manage-actions');
  const act = (work, message) => work().then(async () => {
    await refreshThemes([t.id]);
    toast(message);
    redraw();
  }).catch(err => toast(err.message));
  actions.append(button('Rename', 'edit', 'button button-secondary', () => renamePopup(t, () => act(() => Promise.resolve(), `Renamed ${t.name}`))));
  if (t.owner) {
    actions.append(button('Make public', '', 'button button-secondary', () => act(() => write([{set: t.id, cells: {added_by: ''}}]), `${t.name} is public`)));
  } else if (t.key !== 'plain') {
    actions.append(button('Make private', '', 'button button-secondary', () => act(() => write([{set: t.id, cells: {added_by: me().id}}]), `${t.name} is private to you`)));
  }
  if (t.owner || t.key !== 'plain') {
    actions.append(button('Delete', 'trash', 'button button-secondary theme-manage-delete', () => {
      if (!confirm(deleteThemeWords(t))) {
        return;
      }
      removeTheme(t.id).then(() => {
        toast(`Deleted ${t.name}`);
        redraw();
      }).catch(err => toast(err.message));
    }));
  }
  row.append(picture, words, actions);
  return row;
}

function matches(t, words) {
  const text = [t.name, t.blurb, t.key, t.owner || 'public'].join(' ').toLowerCase();
  return words.every(w => text.includes(w));
}

export function openThemeManager() {
  const list = el('ul', 'theme-manage-list');
  const search = el('input', 'theme-manage-search');
  search.type = 'search';
  search.placeholder = 'Search themes by name, blurb or owner…';
  search.setAttribute('aria-label', 'Search themes');
  let rows = [];
  const show = () => {
    const words = search.value.toLowerCase().split(/\s+/).filter(Boolean);
    const shown = rows.filter(t => matches(t, words));
    list.replaceChildren(...shown.map(t => themeRow(t, redraw)));
    if (!rows.length) {
      list.append(el('li', 'theme-manage-empty', 'Gather has no themes yet. Upload one with Upload a theme.'));
    } else if (!shown.length) {
      list.append(el('li', 'theme-manage-empty', `No theme matches “${search.value.trim()}”.`));
    }
  };
  const redraw = () => allThemes().then(all => {
    rows = all;
    show();
  }).catch(err => toast(err.message));
  search.addEventListener('input', show);
  const head = el('div', 'theme-manage-head');
  head.append(
    el('p', 'theme-hint', 'Every theme on Gather: the site’s own, which every host picks from, and the ones hosts uploaded for themselves. A theme you upload starts private to you; make it public here. Plain is what Create Your Own starts from, so it stays public.'),
    button('Upload a theme', 'upload', 'button', () => themeUpload(id => {
      toast('Uploaded. It’s private to you until you make it public.');
      redraw().then(() => list.querySelector(`[data-theme="${id}"]`)?.scrollIntoView({block: 'nearest'}));
    })),
  );
  openModal('Manage themes', [head, search, list], {wide: true});
  redraw();
}
