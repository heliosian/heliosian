import {state, model, isStudent, isStaff, kidsOf, adultsOf, gradeOf, classroomOf, groupById, photosOf, photoUrl, thumbOf, pronunciationUrl, familyName, familyShortName, canEditFamily, gradePath, classroomPath} from '../state.js';
import {firstName, copyButton, pronouncePill, contactRow} from '../dom.js';
import {el, svg, link, iconButton, iconLink, editToggle} from '/elements.js';
import {myFamily, familyLink} from '../families.js';
import {personByKey, personLink, photoOrInitials, personPhotoUrl, roleWithPronouns, gradeChain} from '../people.js';
import {familyPhotoNeedsUpdate, staleItems, todoChecklist} from '../stale.js';
import {saveCells, editPencil, fieldEditor, uploadIcon, uploadPhoto, pronounceEditor} from '../edit.js';
import {openPhotoLightbox, cropBadge, cropped, familyPhotoMenu, togglePhotoMenu} from '../photos.js';
import {fromURL, peopleCrumbs, breadcrumbs} from '../crumbs.js';
import {render, notFound, trail} from '/router.js';

export function familyDetailChip(text, color, href) {
  const chip = href ? link(href, 'role-label', text) : el('div', 'role-label', text);
  if (color) {
    chip.style.background = `color-mix(in srgb, ${color} 20%, white)`;
    chip.style.color = color;
  }
  return chip;
}

function familyCardRow(p, subtitle) {
  const row = link(personLink(p), 'fcard-row');
  row.append(photoOrInitials(personPhotoUrl(p), p.name_show, 'fcard-avatar'));
  const info = el('div', 'fcard-info');
  info.append(el('div', 'fcard-name', p.name_show));
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

function photoUpload(family, status) {
  return uploadIcon('camera', 'Upload family photo', 'image/*', file => uploadPhoto({group: family.id}, file, status));
}

export function familyCard(p, family) {
  const card = el('div', 'detail-card fcard');
  card.append(el('h2', 'fcard-title', familyName(family)));

  const grid = el('div', 'fcard-grid');
  const left = el('div');
  const editable = canEditFamily(family);
  const showFamilyPhotoEdit = editable && familyPhotoNeedsUpdate(family);
  const nagFamilyPhoto = showFamilyPhotoEdit && isStudent(p);
  const photoWrap = el('div', 'photo-wrap' + (nagFamilyPhoto ? ' needs-update' : ''));
  const status = el('div', 'media-status');
  const photo = photosOf(family.id)[0];
  if (photo) {
    const img = el('img', 'fcard-photo');
    img.src = thumbOf(photo);
    img.alt = '';
    if (cropped(photo)) {
      photoWrap.append(cropBadge());
    }
    if (editable) {
      const menu = familyPhotoMenu(photo, status);
      img.addEventListener('click', () => togglePhotoMenu(menu));
      photoWrap.append(img, menu);
    } else {
      img.addEventListener('click', () => openPhotoLightbox(photoUrl(photo)));
      photoWrap.append(img);
    }
  } else {
    photoWrap.append(photoOrInitials(null, familyName(family), 'fcard-photo fcard-photo-empty'));
  }
  left.append(photoWrap);
  if (showFamilyPhotoEdit) {
    if (nagFamilyPhoto) {
      status.textContent = 'Add your family photo for the new year';
    }
    photoWrap.append(photoUpload(family, status));
  }
  if (editable) {
    left.append(status);
  }
  if (family.description) {
    left.append(el('div', 'fcard-caption', family.description));
  }
  grid.append(left);

  const right = el('div');
  const kidsList = kidsOf(family);
  if (kidsList.length && !(isStudent(p) && kidsList.length === 1 && kidsList[0].id === p.id)) {
    right.append(el('div', 'fcard-section-header', 'Children'));
    for (const kid of kidsList) {
      if (isStudent(p) && kid.id === p.id) {
        continue;
      }
      right.append(familyCardRow(kid, gradeChain(kid)));
    }
  }
  const adults = adultsOf(family).filter(a => a.id !== p.id);
  if (adults.length) {
    right.append(el('div', 'fcard-section-header', 'Other Family Members'));
    for (const adult of adults) {
      right.append(familyCardRow(adult, roleWithPronouns(adult)));
    }
  }
  const seeChip = link(familyLink(family), 'fcard-see-chip');
  seeChip.append(el('span', '', `See ${familyName(family)}`), svg('chevron-right'));
  right.append(seeChip);
  grid.append(right);
  card.append(grid);
  return card;
}

export function familyPage(key) {
  const page = document.createDocumentFragment();
  const family = groupById[key];
  if (!family || family.kind !== 'family') {
    return notFound('That family');
  }
  const editable = canEditFamily(family);
  const editing = editable && state.editing === key;
  const shortName = familyShortName(family);
  const mine = myFamily() === family;
  let crumbs = [['People', '/people'], [shortName, null], ['Family', null]];
  const from = fromURL();
  if (mine) {
    crumbs = [['My Family', '/my-family'], ['Family', null]];
  } else if (from) {
    const rseg = from.pathname.split('/').filter(Boolean).map(decodeURIComponent);
    const back = from.pathname + from.search;
    const rsegPerson = rseg[0] === 'people' && rseg[1] ? personByKey(rseg[1]) : undefined;
    if (rsegPerson) {
      const peopleBack = trail()[1];
      const origin = peopleBack && peopleBack.startsWith('/people') && !peopleBack.startsWith('/people/') ? peopleCrumbs(new URL(peopleBack, location.origin)) : [['People', '/people']];
      crumbs = [...origin, [rsegPerson.name_show, back], ['Family', null]];
    } else if (rseg[0] === 'people' && !rseg[1]) {
      crumbs = [...peopleCrumbs(from), [shortName, null], ['Family', null]];
    } else if (rseg[0] === 'map') {
      crumbs = [['Map', back], [shortName, null], ['Family', null]];
    }
  }
  let toggle = null;
  if (editable) {
    toggle = editToggle('Edit Family', editing, () => {
      state.editing = editing ? '' : key;
      render();
    });
  }
  page.append(breadcrumbs(crumbs, null, toggle));

  if (mine) {
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
  const photo = photosOf(family.id)[0];
  if (photo) {
    const img = el('img', 'detail-photo');
    img.src = photoUrl(photo);
    img.alt = '';
    if (cropped(photo)) {
      wrap.append(cropBadge());
    }
    if (editable) {
      const menu = familyPhotoMenu(photo, status);
      img.addEventListener('click', () => togglePhotoMenu(menu));
      wrap.append(img, menu);
    } else {
      img.addEventListener('click', () => openPhotoLightbox(photoUrl(photo)));
      wrap.append(img);
    }
  } else {
    wrap.append(photoOrInitials(null, familyName(family), 'detail-photo detail-photo-empty'));
  }
  left.append(wrap);
  if (editing) {
    wrap.append(photoUpload(family, status));
  }
  if (editable) {
    left.append(status);
  }
  if (family.description || editing) {
    const captionRow = el('div', 'family-caption', family.description || 'Add a caption');
    left.append(captionRow);
    if (editing) {
      const captionPencil = editPencil('Edit photo caption');
      captionRow.append(captionPencil);
      captionPencil.addEventListener('click', () => fieldEditor(captionRow, captionPencil, {
        current: family.description || '',
        submit: (value, status) => saveCells(family.id, {description: value}, status),
      }));
    }
  }
  grid.append(left);

  const right = el('div');
  const kids = kidsOf(family);
  const adults = adultsOf(family);
  const grades = [...new Map(kids.map(gradeOf).filter(Boolean).map(g => [g.id, g])).values()];
  const homerooms = [...new Map(kids.map(classroomOf).filter(Boolean).map(c => [c.id, c])).values()];
  const topRow = el('div', 'detail-top');
  const chipRow = el('div', 'chip-row');
  for (const g of grades) {
    chipRow.append(familyDetailChip(g.name, g.color, gradePath(g)));
  }
  for (const h of homerooms) {
    chipRow.append(familyDetailChip(h.name, h.color, classroomPath(h)));
  }
  if (adults.some(isStaff)) {
    const staffChip = link('/staff', 'role-label role-label-staff', 'Staff');
    chipRow.append(staffChip);
  }
  topRow.append(chipRow);
  right.append(topRow);
  const nameHeader = el('h1', 'detail-name');
  nameHeader.append(el('span', '', familyName(family)));
  if (family.pronunciation) {
    nameHeader.append(pronouncePill(pronunciationUrl(family), shortName));
  }
  right.append(nameHeader);
  const firsts = [...kids, ...adults].map(m => firstName(m.name_show));
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
  if (editing) {
    right.append(el('div', 'pronounce-label', 'How do I pronounce this?'));
    if (family.pronunciation) {
      const audio = el('audio', 'pronounce-player');
      audio.controls = true;
      audio.preload = 'metadata';
      audio.src = pronunciationUrl(family);
      right.append(audio);
    }
    right.append(pronounceEditor({group: family.id}, family.id, Boolean(family.pronunciation)));
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

export function myFamilyPage() {
  const family = myFamily();
  if (!family) {
    return notFound('Your family');
  }
  history.replaceState(history.state, '', '/families/' + encodeURIComponent(family.id));
  return familyPage(family.id);
}
