import {state, isAdmin, allRoles, sortByStart, rolePath, activityPath} from '../state.js';
import {el, link, svg, avatar, searchBox} from '../dom.js';
import {setTitle} from '../chrome.js';
import {activityRow} from '../cards.js';

let query = '';

function denied() {
  const page = el('div', 'list-page');
  page.append(el('h1', '', 'Admin access required'));
  return page;
}

function personRow(p) {
  const row = link(`/people/${encodeURIComponent(p.email)}`, 'row is-link');
  row.append(avatar(p, 'thumb small'));
  const body = el('div', 'row-body');
  body.append(el('div', 'label', `Activities: ${p.activities}; co-chairing: ${p.coChairing}`));
  body.append(el('div', 'row-title', p.name), el('div', 'row-text', p.email));
  row.append(body);
  const chevron = svg('chevron');
  chevron.classList.add('chevron');
  row.append(chevron);
  return row;
}

export function peoplePage() {
  setTitle('People');
  if (!isAdmin()) {
    return denied();
  }
  const page = el('div', 'list-page');
  const head = el('div', 'list-head');
  head.append(el('h1', '', 'People'));
  const panel = el('div', 'panel');
  const render = () => {
    panel.replaceChildren();
    const shown = (state.model.people || []).filter(p => `${p.name} ${p.email}`.toLowerCase().includes(query));
    for (const p of shown) {
      panel.append(personRow(p));
    }
    if (!shown.length) {
      panel.append(el('div', 'panel-empty', 'Nobody matches.'));
    }
  };
  head.append(searchBox('Search', q => {
    query = q;
    render();
  }, true));
  page.append(head, panel);
  render();
  return page;
}

export function personPage(email) {
  if (!isAdmin()) {
    return denied();
  }
  const person = (state.model.people || []).find(p => p.email === email);
  setTitle(person ? person.name : email);
  const page = el('div', 'list-page');
  const back = el('div', 'crumb');
  const backLink = link('/people', '');
  backLink.append(svg('back'));
  back.append(backLink, link('/people', '', 'People'), el('span', '', '/'), el('span', 'current', person ? person.name : email));
  page.append(back, el('h1', '', person ? person.name : email), el('div', 'row-text', email));
  const rows = [];
  for (const act of state.model.activities) {
    for (const v of act.volunteers) {
      if (v.email === email) {
        rows.push({act, role: null, v});
      }
    }
    for (const role of allRoles(act)) {
      for (const v of role.volunteers) {
        if (v.email === email) {
          rows.push({act, role, v});
        }
      }
    }
  }
  const years = [...new Set(rows.map(r => r.act.year))].sort().reverse();
  for (const year of years) {
    page.append(el('div', 'section-title', year));
    const panel = el('div', 'panel');
    const items = rows.filter(r => r.act.year === year);
    const ordered = sortByStart(items.map(r => ({...(r.role || r.act), _row: r}))).map(n => n._row);
    for (const r of ordered) {
      panel.append(activityRow(r.act, {role: r.role, href: r.role ? rolePath(r.act, r.role) : activityPath(r.act), noJoin: true,
        actions: [el('span', 'badge ' + (r.v.position === 'Co-Chair' ? 'chair' : r.v.position === 'Open to Co-Chair' ? 'open-chair' : ''), r.v.position)]}));
    }
    page.append(panel);
  }
  if (!rows.length) {
    const panel = el('div', 'panel');
    panel.append(el('div', 'panel-empty', 'No sign-ups.'));
    page.append(panel);
  }
  return page;
}
