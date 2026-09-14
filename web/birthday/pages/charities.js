import {state, isAdmin, charityPath, staffPath} from '../state.js';
import {el, link, svg, button, tabs, pageHead} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {charityRow, emptyPanel} from '../cards.js';
import {openCharity} from '../edit.js';

let tab = 'allowed';
let query = '';

function list() {
  const rows = state.model.charities
    .filter(c => c.allowed === (tab === 'allowed'))
    .filter(c => !query || `${c.name} ${c.about || ''} ${c.ein || ''}`.toLowerCase().includes(query));
  if (!rows.length) {
    return emptyPanel(query ? 'Nothing matches.' : 'None yet.');
  }
  const panel = el('div', 'panel');
  for (const c of rows) {
    panel.append(charityRow(c, [{icon: 'edit', label: 'Edit', onClick: () => openCharity(c)}]));
  }
  return panel;
}

export function charitiesPage() {
  setTitle('Charities');
  const page = el('div', 'list-page');
  page.append(pageHead('Charities', [button('Add', 'plus', 'button', () => openCharity(null))]));
  const body = el('div');
  const render = () => {
    const count = allowed => state.model.charities.filter(c => c.allowed === allowed).length;
    body.replaceChildren(tabs([{key: 'allowed', label: 'Allowed', count: count(true)}, {key: 'prohibited', label: 'Prohibited', count: count(false)}], tab, key => {
      tab = key;
      render();
    }));
    const wrap = el('div', 'list');
    wrap.append(list());
    body.append(wrap);
  };
  query = '';
  setSearch('Search charities…', q => {
    query = q;
    body.querySelector('.list').replaceChildren(list());
  });
  render();
  page.append(body);
  return page;
}

export function charityPage(c) {
  setTitle(c.name);
  const page = el('div', 'list-page');
  const nav = el('div', 'crumb');
  const back = link('/charities', '');
  back.append(svg('back'));
  nav.append(back, link('/charities', '', 'Charities'), el('span', '', '/'), el('span', 'current', c.name));
  page.append(nav);
  const head = el('div', 'page-head');
  const body = el('div', 'page-head-main');
  body.append(el('h1', 'page-title', c.name));
  if (!c.allowed) {
    body.append(el('div', 'label').appendChild(el('span', 'need', `Not allowed: ${c.whyNotAllowed || 'no reason given'}`)).parentNode);
  }
  if (c.about) {
    body.append(el('p', 'row-text', c.about));
  }
  head.append(body, button('Edit', 'edit', 'button', () => openCharity(c)));
  page.append(head);
  const facts = el('div', 'facts');
  const linkFact = el('div', 'fact');
  const anchor = el('a', 'fact-value', c.donationLink.replace(/^https?:\/\//, ''));
  anchor.href = c.donationLink;
  anchor.target = '_blank';
  anchor.rel = 'noopener';
  linkFact.append(el('div', 'fact-label', 'Donation Link'), anchor);
  facts.append(linkFact);
  if (c.addedOn) {
    const added = el('div', 'fact');
    added.append(el('div', 'fact-label', 'Added'), el('div', 'fact-value', c.addedOn));
    facts.append(added);
  }
  page.append(facts);
  const chosen = state.model.staff.filter(sv => (sv.donation && sv.donation.charity === c.name) || (sv.lastDonation && sv.lastDonation.charity === c.name));
  if (chosen.length) {
    page.append(el('h2', 'section', 'Chosen by'));
    const panel = el('div', 'panel');
    for (const sv of chosen) {
      const row = link(staffPath(sv), 'row is-link');
      const rb = el('div', 'row-body');
      rb.append(el('div', 'row-title', sv.name), el('div', 'row-text', sv.donation && sv.donation.charity === c.name ? 'This year' : 'Last year'));
      row.append(rb);
      const chevron = svg('chevron');
      chevron.classList.add('chevron');
      row.append(chevron);
      panel.append(row);
    }
    page.append(panel);
  }
  if (!isAdmin()) {
    page.append(el('div', 'footnote', 'Only an admin can mark a charity as not allowed or remove it.'));
  }
  return page;
}

export function charityLink(c) {
  return link(charityPath(c), '', c.name);
}
