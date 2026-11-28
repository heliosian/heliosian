import {el} from '/elements.js';
import {themed} from '/themes.js';
import {render as markdown} from '/markdown.js';

function paperOf(look, item) {
  const sheet = themed(el('article', 'invite-sheet'), {...look, cardPicture: ''});
  if (item.background) {
    sheet.style.setProperty('--paper', item.background);
    sheet.style.setProperty('--card-art', 'none');
  }
  if (item.pictureUrl) {
    sheet.style.setProperty('--card-art', `url("${item.pictureUrl}") center / cover no-repeat`);
    sheet.style.setProperty('--art-mask', 'none');
    sheet.style.setProperty('--art-opacity', '1');
    sheet.style.setProperty('--frame-inset', '0');
  }
  const paper = el('div', 'invite-sheet-paper');
  const art = el('div', 'card-art');
  art.setAttribute('aria-hidden', 'true');
  paper.append(art);
  sheet.append(paper);
  return {sheet, paper};
}

export function openSheet(look, item, ...extra) {
  const {sheet, paper} = paperOf(look, item);
  const text = el('div', 'markdown invite-sheet-text');
  text.append(markdown(item.body || ''));
  const inner = el('div', 'invite-sheet-inner');
  inner.append(el('h2', 'invite-sheet-title', item.title), el('div', 'card-rule'), text);
  paper.append(...extra, inner);
  return sheet;
}

function peekSheet(look, item, below, depth, open) {
  const {sheet, paper} = paperOf(look, item);
  sheet.classList.add('is-folded', below ? 'is-below' : 'is-above');
  sheet.style.setProperty('--i', String(depth));
  const cover = el('button', 'invite-sheet-cover');
  cover.type = 'button';
  cover.append(el('span', 'invite-sheet-title', item.title));
  cover.addEventListener('click', () => open(item));
  paper.append(cover);
  if (below) {
    paper.append(el('span', 'invite-sheet-fold'));
  }
  return sheet;
}

export function fillDeck(look, deck, focus, above, below, open) {
  const at = Math.max(0, deck.findIndex(item => item.id === focus));
  above.replaceChildren(...deck.slice(0, at).map((item, j) => peekSheet(look, item, false, at - 1 - j, open)));
  below.replaceChildren(...deck.slice(at + 1).map((item, j) => peekSheet(look, item, true, j, open)));
  return deck[at];
}
