import {state, byEmail, colors} from '../state.js';
import {el, svg, thumbUrl, firstName, iconButton, copyButton, pronouncePill, contactRow, withFrom, slugify} from '../dom.js';
import {myFamilyKey, familyLink} from '../families.js';
import {personByKey, personLink, photoOrInitials, personPhotoUrl, roleWithPronouns, gradeChain} from '../people.js';
import {familyPhotoNeedsUpdate, staleItems, todoChecklist} from '../stale.js';
import {submitField, editPencil, fieldEditor, uploadIcon, pronounceEditor} from '../edit.js';
import {openPhotoLightbox, cropBadge, familyPhotoMenu, togglePhotoMenu} from '../photos.js';
import {fromURL, breadcrumbs} from '../crumbs.js';
import {resetMain, finishRender} from '../chrome.js';

// A grade/homeroom chip tinted with that grade or classroom's own
// admin-configured color (light background, solid text - same pairing as
// .role-label-student/parent/staff) instead of the flat default role-label
// styling, so it visually matches ringColorFor/applyRingColor's use of the
// same color elsewhere (person cards, hover). Falls back to the default
// role-label look if that grade/classroom has no color configured. href, when
// given, makes it a link to that grade/classroom/staff page (same href shape
// gradeCard/classroomCard and the Staff nav item use elsewhere).
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
    for (const [name, label, scheme] of [['message', 'Text', 'sms:'], ['phone', 'Call', 'tel:']]) {
      actions.append(iconButton(name, label, e => {
        e.preventDefault();
        e.stopPropagation();
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
  const familyEditable = family.key === myFamilyKey() || state.model.superEdit;
  const showFamilyPhotoEdit = familyEditable && familyPhotoNeedsUpdate(family);
  // The "update for the new year" nagging (dashed outline + reminder text) is meant
  // for students, same as personal photos/facts - an adult visiting their own page
  // still gets the camera icon to make uploading easy, just without the nag.
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
    photoWrap.append(uploadIcon('camera', 'Upload family photo', 'image/*', 'family', family.key, 'photo', status));
  }
  if (familyEditable) {
    left.append(status);
  }
  if (family.photoCaption) {
    left.append(el('div', 'fcard-caption', family.photoCaption));
  }
  grid.append(left);

  const right = el('div');
  const kidsList = (family.kidEmails || []).map(e => byEmail[e]).filter(Boolean);
  if (kidsList.length && !(p.isStudent && kidsList.length === 1 && kidsList[0].email === p.email)) {
    right.append(el('div', 'fcard-section-header', 'Children'));
    for (const kid of kidsList) {
      if (p.isStudent && kid.email === p.email) {
        continue;
      }
      right.append(familyCardRow(kid, gradeChain(kid)));
    }
  }
  const adults = (family.adultEmails || []).map(e => byEmail[e]).filter(Boolean).filter(a => a.email !== p.email);
  if (adults.length) {
    right.append(el('div', 'fcard-section-header', 'Other Family Members'));
    for (const adult of adults) {
      right.append(familyCardRow(adult, roleWithPronouns(adult)));
    }
  }
  const seeChip = el('a', 'fcard-see-chip');
  seeChip.href = familyLink(family.key);
  const shortName = (family.name || '').replace(/ Family$/, '');
  seeChip.append(el('span', '', `See ${shortName} Family`), svg('chevron-right'));
  right.append(seeChip);
  grid.append(right);
  card.append(grid);
  return card;
}

let familyEdit = null;

export function renderFamilyDetail(key) {
  const main = resetMain();
  const family = state.model.families[key];
  if (!family) {
    main.append(el('div', 'empty', 'Not found.'));
    return;
  }
  const editable = key === myFamilyKey() || state.model.superEdit;
  const editing = editable && familyEdit === key;
  const shortName = (family.name || '').replace(/ Family$/, '');
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
  main.append(breadcrumbs(crumbs));

  if (editable) {
    const items = staleItems();
    if (items.length) {
      const wrap = el('div', 'container');
      wrap.append(todoChecklist(items));
      main.append(wrap);
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
  if (editing) {
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
        submit: (value, status) => submitField(key, 'family-photo-caption', value, status),
      }));
    }
  }
  grid.append(left);

  const right = el('div');
  const kids = (family.kidEmails || []).map(e => byEmail[e]).filter(Boolean);
  const adults = (family.adultEmails || []).map(e => byEmail[e]).filter(Boolean);
  const grades = [...new Set(kids.map(k => k.grade).filter(Boolean))];
  const homerooms = [...new Set(kids.map(k => k.classroom).filter(Boolean))];
  const topRow = el('div', 'detail-top');
  const chipRow = el('div', 'chip-row');
  // Each kid's grade and homeroom as its own chip, colored with that grade's
  // or classroom's admin-configured color (see colors in state.js) rather
  // than one "GRADE 3, GRADE 6" chip lumping every kid's grade into a single
  // label - a family with kids spread across grades/homerooms shows one clear
  // chip per grade and per homeroom instead.
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
  if (editable) {
    const topActions = el('div', 'detail-top-actions');
    const toggle = editing
      ? el('button', 'media-button edit-toggle', 'Done')
      : iconButton('pencil', 'Edit info', () => {
        familyEdit = key;
        renderFamilyDetail(key);
        finishRender();
      });
    if (editing) {
      toggle.addEventListener('click', () => {
        familyEdit = null;
        renderFamilyDetail(key);
        finishRender();
      });
    }
    topActions.append(toggle);
    topRow.append(topActions);
  }
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
      iconButton('map', 'Map', 'https://maps.google.com/?q=' + encodeURIComponent(family.address)),
      copyButton(family.address),
    ]));
  }
  if (editing) {
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
  main.append(content);

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
  main.append(band);

  if (location.hash) {
    const target = document.querySelector(location.hash);
    if (target) {
      target.scrollIntoView({block: 'center'});
    }
  }
}
