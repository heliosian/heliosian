import {el, button} from '/elements.js';
import {openModal} from '/modal.js';
import {field, text, textarea} from '/form.js';
import {navigate, load} from '/router.js';
import {lookTheme} from '/themes.js';
import {textTools, indentLists} from '/texttools.js';
import {savePage, removePage, setPagePicture, clearPagePicture} from './state.js';
import {pictureInput, firstHex} from './editor.js';

function backgroundField(inv, current) {
  let own = current;
  const input = el('input', 'color-swatch');
  input.type = 'color';
  const reset = button('Use the card’s', '', 'link-button', () => {
    own = '';
    sync();
  });
  const sync = () => {
    input.value = own || firstHex(lookTheme(inv.look).card.background);
    reset.hidden = !own;
  };
  input.addEventListener('input', () => {
    own = input.value;
    reset.hidden = false;
  });
  sync();
  const row = el('div', 'color-row');
  row.append(input, reset);
  return {wrap: field('Background color', row, 'Blank uses the card’s background.'), value: () => own};
}

function pictureSlot(page) {
  if (!page) {
    const wrap = el('div', 'field picture-field');
    wrap.append(el('span', '', 'Background picture'), el('small', '', 'Add the page first, then add a picture here.'));
    return wrap;
  }
  return pictureInput('Background picture', page.pictureUrl, file => setPagePicture(page, file), () => clearPagePicture(page), () => load());
}

export function openPageEditor(inv, page, key) {
  const title = text(page ? page.title : '', {required: true, placeholder: 'Schedule, What to bring, Directions…'});
  const body = textarea(page ? page.body || '' : '', 12);
  body.placeholder = 'What guests see on this page.';
  indentLists(body);
  const bodyBox = el('div', 'text-box');
  bodyBox.append(textTools(body), body);
  const background = backgroundField(inv, page ? page.background : '');
  const base = `/g/${encodeURIComponent(key)}`;
  let id = '';
  openModal(page ? 'Edit page' : 'Add a page', [
    field('Title', title, 'Its name in the invitation’s sidebar.', true),
    field('Page', bodyBox, 'Markdown: **bold**, *italic*, - a list, [words](https://a.link).'),
    background.wrap,
    pictureSlot(page),
  ], {
    wide: true,
    saveLabel: page ? 'Save' : 'Add page',
    submit: async () => {
      id = await savePage(inv, page, {title: title.value.trim(), body: body.value, background: background.value()});
      return id;
    },
    afterSave: () => navigate(`${base}/p/${id}`),
    onDelete: page ? () => removePage(page) : null,
    deleteLabel: 'Remove page',
    confirmDelete: 'Remove this page from the invitation?',
    afterDelete: () => navigate(base),
  });
}
