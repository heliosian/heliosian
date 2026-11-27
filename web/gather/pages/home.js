import {el, link, button} from '/elements.js';
import {pageHead, setTitle, setSearch} from '/shell.js';
import {state, dayWords, hourWords} from '../state.js';
import {miniEnvelope, andList} from '/envelope.js';
import {openEditor} from '../editor.js';

function wordsOf(inv) {
  return [inv.name, inv.subtitle, inv.description, inv.place, inv.location, inv.hostedBy, ...inv.hostNames, dayWords(inv), hourWords(inv), inv.pageWords].join(' ').toLowerCase();
}

const tiles = new WeakMap();

function marked(text, terms) {
  const out = document.createDocumentFragment();
  if (!terms.length) {
    out.append(text);
    return out;
  }
  const pattern = new RegExp(`(${terms.map(t => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})`, 'gi');
  for (const [i, piece] of text.split(pattern).entries()) {
    out.append(i % 2 ? el('mark', 'search-hit', piece) : piece);
  }
  return out;
}

function snippet(text, term) {
  const flat = text.replace(/[*#_>`]/g, '').replace(/\s+/g, ' ').trim();
  const at = flat.toLowerCase().indexOf(term);
  const from = Math.max(0, at - 40);
  const to = Math.min(flat.length, at + term.length + 50);
  return (from > 0 ? '…' : '') + flat.slice(from, to) + (to < flat.length ? '…' : '');
}

function tile(inv) {
  const a = link(inv.path, 'invite-tile');
  a.dataset.words = wordsOf(inv);
  const words = el('span', 'invite-tile-words');
  const lines = [];
  const line = (cls, text) => {
    const span = el('span', cls, text);
    lines.push({span, text});
    words.append(span);
  };
  line('invite-tile-title', inv.name);
  const by = inv.hostedBy || andList(inv.hostNames);
  if (!inv.host && by) {
    line('invite-tile-host', 'Hosted by ' + by);
  }
  line('invite-tile-day', inv.status === 'cancelled' ? 'Cancelled' : dayWords(inv, true));
  const match = el('span', 'invite-tile-match');
  match.hidden = true;
  words.append(match);
  a.append(miniEnvelope(inv.look, inv.name), words);
  const elsewhere = [inv.subtitle, inv.description, inv.place, inv.location, hourWords(inv), dayWords(inv), inv.hostedBy, ...inv.hostNames, inv.pageWords].filter(Boolean);
  tiles.set(a, {lines, match, elsewhere});
  return a;
}

function highlight(a, terms) {
  const {lines, match, elsewhere} = tiles.get(a);
  for (const {span, text} of lines) {
    span.replaceChildren(marked(text, terms));
  }
  const seen = lines.map(l => l.text.toLowerCase()).join(' ');
  const missing = terms.filter(term => !seen.includes(term));
  const text = missing.length ? elsewhere.find(t => t.toLowerCase().includes(missing[0])) : '';
  match.hidden = !text;
  if (!text) {
    match.replaceChildren();
    return;
  }
  match.replaceChildren(marked(snippet(text, missing[0]), terms));
}

function section(title, invites, empty) {
  const wrap = el('section', 'home-section');
  wrap.append(el('h2', 'home-section-title', title));
  if (!invites.length) {
    wrap.append(el('p', 'invite-empty', empty));
    return wrap;
  }
  const grid = el('div', 'invite-grid');
  grid.append(...invites.map(tile));
  wrap.append(grid);
  return wrap;
}

function filter(page, nothing, query) {
  const terms = query.split(/\s+/).filter(Boolean);
  let shown = 0;
  for (const t of page.querySelectorAll('.invite-tile')) {
    t.hidden = !terms.every(term => t.dataset.words.includes(term));
    if (!t.hidden) {
      highlight(t, terms);
      shown += 1;
    }
  }
  for (const s of page.querySelectorAll('.home-section')) {
    const tiles = s.querySelectorAll('.invite-tile');
    s.hidden = terms.length > 0 && tiles.length > 0 && [...tiles].every(t => t.hidden);
    const blank = s.querySelector('.invite-empty');
    if (blank) {
      s.hidden = terms.length > 0;
    }
  }
  nothing.hidden = !terms.length || shown > 0;
  nothing.textContent = `No invitations match “${query}”.`;
}

export function homePage() {
  setTitle('Helios Gather');
  const page = el('div', 'gather-home');
  page.append(pageHead('Invitations', [button('New invitation', 'plus', 'button', () => openEditor(null))]));
  const nothing = el('p', 'invite-empty');
  nothing.hidden = true;
  page.append(
    nothing,
    section('Invited to', state.model.invited, 'No invitations yet. When someone sends you one, it arrives here.'),
    section('Hosting', state.model.hosting, 'Nothing you’re hosting yet. Make an invitation and choose its design.'),
  );
  setSearch('Search invitations…', query => filter(page, nothing, query));
  return page;
}
