import {el, link, imageThumb} from '/elements.js';
import {searchInput} from '/shell.js';
import {parseWhen} from '/datecard.js';

let rows = [];
let active = -1;
const dayFormat = new Intl.DateTimeFormat('en-US', {month: 'short', day: 'numeric'});

export function resultDay(start) {
  const when = parseWhen(start);
  return when ? dayFormat.format(when.date) : 'All Year';
}

function menu() {
  return document.querySelector('#search-results');
}

export function closeResults() {
  const node = menu();
  node.hidden = true;
  node.replaceChildren();
  rows = [];
  active = -1;
}

function resultRow(item, earlier) {
  const row = link(item.href, 'search-row' + (earlier ? ' is-earlier' : ''));
  row.append(imageThumb(item.image, item.imageTitle || item.title, 'search-row-pic'));
  row.append(el('span', 'search-row-day', item.day));
  const body = el('span', 'search-row-body');
  body.append(el('span', 'search-row-title', item.title));
  if (item.line) {
    body.append(el('span', 'search-row-line', item.line));
  }
  row.append(body);
  row.addEventListener('mousedown', e => e.preventDefault());
  row.addEventListener('click', closeResults);
  row.addEventListener('mouseenter', () => setActive(rows.indexOf(row)));
  rows.push(row);
  return row;
}

export function showResults(current, earlierLabel, earlier) {
  closeResults();
  const node = menu();
  if (!current.length && !earlier.length) {
    node.append(el('div', 'search-empty', 'Nothing matches.'));
    node.hidden = false;
    return;
  }
  for (const item of current) {
    node.append(resultRow(item, false));
  }
  if (earlier.length) {
    const divider = el('div', 'search-divider');
    divider.append(el('span', '', earlierLabel));
    node.append(divider);
  }
  for (const item of earlier) {
    node.append(resultRow(item, true));
  }
  node.hidden = false;
}

function setActive(index) {
  active = index;
  rows.forEach((row, i) => row.classList.toggle('is-active', i === index));
  if (index >= 0) {
    rows[index].scrollIntoView({block: 'nearest'});
  }
}

function onKey(e) {
  if (menu().hidden) {
    return;
  }
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault();
    if (!rows.length) {
      return;
    }
    const step = e.key === 'ArrowDown' ? 1 : -1;
    setActive((active + step + rows.length) % rows.length);
  } else if (e.key === 'Enter') {
    if (active >= 0) {
      e.preventDefault();
      rows[active].click();
    }
  } else if (e.key === 'Escape') {
    e.preventDefault();
    closeResults();
    searchInput().blur();
  }
}

export function initResults() {
  searchInput().addEventListener('keydown', onKey);
  searchInput().addEventListener('blur', closeResults);
}
