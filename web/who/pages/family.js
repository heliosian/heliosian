import {state, colors, peopleOf} from '../state.js';
import {thumbUrl, firstName, copyButton, pronouncePill, contactRow, withFrom, slugify} from '../dom.js';
import {el, svg, iconButton, iconLink, editToggle} from '/elements.js';
import {myFamilyKey, familyLink} from '../families.js';
import {personByKey, personLink, photoOrInitials, personPhotoUrl, roleWithPronouns, gradeChain} from '../people.js';
import {familyPhotoNeedsUpdate, staleItems, todoChecklist} from '../stale.js';
import {submitEdit, editPencil, fieldEditor, uploadIcon, pronounceEditor} from '../edit.js';
import {openPhotoLightbox, cropBadge, familyPhotoMenu, togglePhotoMenu} from '../photos.js';
import {fromURL, breadcrumbs} from '../crumbs.js';
import {render} from '/router.js';

export function familyDetailChip(text, color, href) {
  const chip = el(href ? 'a' : 'div', 'role-label', text);
  if (href) {
    chip.href = href;
  }
  if (color) {
    chip.style.background = `color-mix(in srgb, ${color} 20%, white)`;
    chip.style.color = color;
  }
  return chip;
}

function familyCardRow(p, subtitle) {
  const row = el('a', 'fcard-row');
  row.href = personLink(p);
  row.append(photoOrInitials(personPhotoUrl(p), p.fullName, 'fcard-avatar'));
  const info = el('div', 'fcard-info');
  info.append(el('div', 'fcard-name', p.fullName));
  if (subtitle) {
    info.append(el('div', 'fcard-sub', subtitle));
  }
  row.append(info);
  if (p.phone) {
    const actions = el('div', 'fcard-actions');
    actions.append(el('span', 'fcard-phone', p.phone));
    for (const [name, label, scheme] of [['chat', 'Text', 'sms:'], ['phone', 'Call', 'tel:']]) {
      actions.append(iconButton(name, label, '', () => {
        location.href = scheme + p.phone;
      }));
    }
    row.append(actions);
  }
  const chev = el('div', 'fcard-chevron');
  chev.append(svg('chevron-right'));
  row.append(chev);
  return row;
}

export function familyCard(p, family) {
  const card = el('div', 'detail-card fcard');
  card.append(el('h2', 'fcard-title', family.name));

  const grid = el('div', 'fcard-grid');
  const left = el('div');
  const familyEditable = family.can['crop-photo'];
  const showFamilyPhotoEdit = family.can.photo && familyPhotoNeedsUpdate(family);
  const nagFamilyPhoto = showFamilyPhotoEdit && p.isStudent;
  const photoWrap = el('div', 'photo-wrap' + (nagFamilyPhoto ? ' needs-update' : ''));
  const status = el('div', 'media-status');
  if (family.photoUrl) {
    const img = el('img', 'fcard-photo');
    img.src = thumbUrl(family.photoUrl);
    img.alt = '';
    if (family.photoUrl !== family.originalPhotoUrl) {
      photoWrap.append(cropBadge());
    }
    if (familyEditable) {
      const menu = familyPhotoMenu(family, status);
      img.addEventListener('click', () => togglePhotoMenu(menu));
      photoWrap.append(img, menu);
    } else {
      img.addEventListener('click', () => openPhotoLightbox(family.originalPhotoUrl || family.photoUrl));
      photoWrap.append(img);
    }
  } else {
    photoWrap.append(photoOrInitials(null, family.name, 'fcard-photo fcard-photo-empty'));
  }
  left.append(photoWrap);
  if (showFamilyPhotoEdit) {
    if (nagFamilyPhoto) {
      status.textContent = 'Add your family photo for the new year';
    }
    photoWrap.append(uploadIcon('camera', 'Upload family photo', 'image/*', 'family', family.id, 'photo', status));
  }
  if (familyEditable) {
    left.append(status);
  }
  if (family.photoCaption) {
    left.append(el('div', 'fcard-caption', family.photoCaption));
  }
  grid.append(left);

  const right = el('div');
  const kidsList = peopleOf(family.kids);
  if (kidsList.length && !(p.isStudent && kidsList.length === 1 && kidsList[0].id === p.id)) {
    right.append(el('div', 'fcard-section-header', 'Children'));
    for (const kid of kidsList) {
      if (p.isStudent && kid.id === p.id) {
        continue;
      }
      right.append(familyCardRow(kid, gradeChain(kid)));
    }
  }
  const adults = peopleOf(family.adults).filter(a => a.id !== p.id);
  if (adults.length) {
    right.append(el('div', 'fcard-section-header', 'Other Family Members'));
    for (const adult of adults) {
      right.append(familyCardRow(adult, roleWithPronouns(adult)));
    }
  }
  const seeChip = el('a', 'fcard-see-chip');
  seeChip.href = familyLink(family.id);
  seeChip.append(el('span', '', `See ${family.name || ''}`), svg('chevron-right'));
  right.append(seeChip);
  grid.append(right);
  card.append(grid);
  return card;
}

let familyEdit = null;

export function familyPage(key) {
  const page = document.createDocumentFragment();
  const family = state.model.families[key];
  if (!family) {
    page.append(el('div', 'empty', 'Not found.'));
    return page;
  }
  const editable = family.can.edit;
  const editing = editable && familyEdit === key;
  const shortName = family.shortName || '';
  let crumbs = [['People', '/people'], [shortName, null], ['Family', null]];
  const from = fromURL();
  if (key === myFamilyKey()) {
    crumbs = [['My Family', '/my-family'], ['Family', null]];
  } else if (from) {
    const rseg = from.pathname.split('/').filter(Boolean).map(decodeURIComponent);
    const back = from.pathname + from.search;
    const rsegPerson = rseg[0] === 'people' && rseg[1] ? personByKey(rseg[1]) : undefined;
    if (rsegPerson) {
      const person = rsegPerson;
      const peopleBack = new URLSearchParams(from.search).get('from');
      const peopleHref = peopleBack && peopleBack.startsWith('/people') && !peopleBack.startsWith('/people/') ? peopleBack : '/people';
      crumbs = [['People', peopleHref], [person.fullName, back], ['Family', null]];
    } else if (rseg[0] === 'people' && !rseg[1]) {
      crumbs = [['People', back], [shortName, null], ['Family', null]];
    } else if (rseg[0] === 'map') {
      crumbs = [['Map', back], [shortName, null], ['Family', null]];
    }
  }
  let toggle = null;
  if (editable) {
    toggle = editToggle('Edit Family', editing, () => {
      familyEdit = editing ? null : key;
      render();
    });
  }
  page.append(breadcrumbs(crumbs, null, toggle));

  if (key === myFamilyKey()) {
    const items = staleItems();
    if (items.length) {
      const wrap = el('div', 'container');
      wrap.append(todoChecklist(items));
      page.append(wrap);
    }
  }

  const content = el('div', 'container detail-content');
  const headerCard = el('div', 'detail-card');
  const grid = el('div', 'detail-grid');
  const left = el('div');
  left.id = 'family-photo';
  const wrap = el('div', 'photo-wrap');
  const status = el('div', 'media-status');
  if (family.photoUrl) {
    const img = el('img', 'detail-photo');
    img.src = family.photoUrl;
    img.alt = '';
    if (family.photoUrl !== family.originalPhotoUrl) {
      wrap.append(cropBadge());
    }
    if (editable) {
      const menu = familyPhotoMenu(family, status);
      img.addEventListener('click', () => togglePhotoMenu(menu));
      wrap.append(img, menu);
    } else {
      img.addEventListener('click', () => openPhotoLightbox(family.originalPhotoUrl || family.photoUrl));
      wrap.append(img);
    }
  } else {
    wrap.append(photoOrInitials(null, family.name, 'detail-photo detail-photo-empty'));
  }
  left.append(wrap);
  if (editing && family.can.photo) {
    wrap.append(uploadIcon('camera', 'Upload family photo', 'image/*', 'family', key, 'photo', status));
  }
  if (editable) {
    left.append(status);
  }
  if (family.photoCaption || editing) {
    const captionRow = el('div', 'family-caption', family.photoCaption || 'Add a caption');
    left.append(captionRow);
    if (editing) {
      const captionPencil = editPencil('Edit photo caption');
      captionRow.append(captionPencil);
      captionPencil.addEventListener('click', () => fieldEditor(captionRow, captionPencil, {
        current: family.photoCaption || '',
        submit: (value, status) => submitEdit('family', key, {photoCaption: value}, status),
      }));
    }
  }
  grid.append(left);

  const right = el('div');
  const kids = peopleOf(family.kids);
  const adults = peopleOf(family.adults);
  const grades = [...new Set(kids.map(k => k.grade).filter(Boolean))];
  const homerooms = [...new Set(kids.map(k => k.classroom).filter(Boolean))];
  const topRow = el('div', 'detail-top');
  const chipRow = el('div', 'chip-row');
  for (const g of grades) {
    chipRow.append(familyDetailChip(g, colors.grades[g], withFrom('/grades/' + slugify(g))));
  }
  for (const h of homerooms) {
    chipRow.append(familyDetailChip(h, colors.classrooms[h], withFrom('/classrooms/' + slugify(h))));
  }
  if (adults.some(a => a.isStaff)) {
    const staffChip = el('a', 'role-label role-label-staff', 'Staff');
    staffChip.href = withFrom('/staff');
    chipRow.append(staffChip);
  }
  topRow.append(chipRow);
  right.append(topRow);
  const nameHeader = el('h1', 'detail-name');
  nameHeader.append(el('span', '', family.name));
  if (family.pronunciationUrl) {
    nameHeader.append(pronouncePill(family.pronunciationUrl, shortName));
  }
  right.append(nameHeader);
  const firsts = [...kids, ...adults].map(m => firstName(m.fullName));
  if (firsts.length) {
    right.append(el('div', 'detail-sub', firsts.join(', ')));
  }
  if (family.address) {
    const addressValue = el('div', 'contact-value');
    addressValue.append(svg('map'), el('span', '', family.address));
    right.append(contactRow(addressValue, [
      iconLink('map', 'Map', 'https://maps.google.com/?q=' + encodeURIComponent(family.address)),
      copyButton(family.address),
    ]));
  }
  if (editing && family.can.pronunciation) {
    right.append(el('div', 'pronounce-label', 'How do I pronounce this?'));
    if (family.pronunciationUrl) {
      const audio = el('audio', 'pronounce-player');
      audio.controls = true;
      audio.preload = 'metadata';
      audio.src = family.pronunciationUrl;
      right.append(audio);
    }
    right.append(pronounceEditor('family', key, !!family.pronunciationUrl));
  }
  grid.append(right);
  headerCard.append(grid);
  content.append(headerCard);
  page.append(content);

  const band = el('div', 'container fcard-wrap');
  const membersCard = el('div', 'detail-card fcard');
  membersCard.append(el('h2', 'fcard-title', 'Family Members'));
  const cols = el('div', 'members-grid');
  const adultsCol = el('div');
  if (adults.length) {
    adultsCol.append(el('div', 'fcard-section-header', 'Adults'));
    for (const a of adults) {
      adultsCol.append(familyCardRow(a, roleWithPronouns(a)));
    }
  }
  const kidsCol = el('div');
  if (kids.length) {
    kidsCol.append(el('div', 'fcard-section-header', 'Children'));
    for (const k of kids) {
      kidsCol.append(familyCardRow(k, gradeChain(k)));
    }
  }
  cols.append(adultsCol, kidsCol);
  membersCard.append(cols);
  band.append(membersCard);
  page.append(band);

  if (location.hash) {
    queueMicrotask(() => {
      const target = document.querySelector(location.hash);
      if (target) {
        target.scrollIntoView({block: 'center'});
      }
    });
  }
  return page;
}
