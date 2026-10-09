import {render, notFound} from '/router.js';
import {viewerId, emailOf, isStudent, isStaff, familiesOf, photosOf, photoUrl, fullOf, pronunciationUrl, classroomOf, classroomPath, groupById, canEdit} from '../state.js';
import {withFrom, firstName, copyButton, pronouncePill, contactRow, aboutMeText, paletteColor} from '../dom.js';
import {el, svg, iconLink, editToggle} from '/elements.js';
import {personByKey, baseRole, gradeChain, photoOrInitials, formatPronouns, familyPhoto} from '../people.js';
import {photoNeedsUpdate, factsNeedUpdate, staleItems, todoChecklist, monthYear} from '../stale.js';
import {saveCells, namesFor, today, editPencil, fieldEditor, uploadIcon, uploadPhoto, pronounceEditor} from '../edit.js';
import {openPhotoLightbox, cropBadge, cropped, photoMenu, togglePhotoMenu, photoGrid} from '../photos.js';
import {fromCrumbs, breadcrumbs} from '../crumbs.js';
import {familyCard} from './family.js';

function tintChip(chip, color) {
  if (color) {
    chip.style.background = `color-mix(in srgb, ${color} 20%, white)`;
    chip.style.color = color;
  }
}

function personSummaryText(p, family) {
  const lines = [p.name_show];
  lines.push(p.pronouns ? `${baseRole(p)} · ${formatPronouns(p.pronouns)}` : baseRole(p));
  lines.push('');
  if (p.phone) {
    lines.push('Phone: ' + p.phone);
  }
  if (emailOf(p)) {
    lines.push('Email: ' + emailOf(p));
  }
  if (family && family.address) {
    lines.push('Address: ' + family.address);
  }
  if (p.facts) {
    lines.push('', 'About: ' + p.facts);
  }
  return lines.join('\n');
}

function legalNameLine(p) {
  if (!p.vc_legal_name || p.vc_legal_name === p.name_show) {
    return null;
  }
  return p.vc_legal_name;
}

let personEdit = null;

export function personPage(key) {
  const page = document.createDocumentFragment();
  const p = personByKey(key);
  if (!p) {
    return notFound('That person');
  }
  const params = new URLSearchParams(location.search);
  const focusFacts = params.get('focus') === 'facts';
  if (params.get('edit') === '1') {
    params.delete('edit');
    params.delete('focus');
    const query = params.toString();
    history.replaceState(null, '', location.pathname + (query ? '?' + query : ''));
    personEdit = p.id;
  }
  const editable = canEdit(p);
  const editing = editable && personEdit === p.id;
  let toggle = null;
  if (editable) {
    toggle = editToggle('Edit Person', editing, () => {
      personEdit = editing ? null : p.id;
      render();
    });
  }
  const origin = fromCrumbs() || [['People', '/people']];
  const crumbsRow = breadcrumbs([...origin, [p.name_show, null]], p.id, toggle);

  const photos = photosOf(p.id);
  const nagPhoto = editable && photoNeedsUpdate(p);
  const showPhotoEdit = editable && photos.length === 0;
  const showFactsEdit = editing || (editable && factsNeedUpdate(p));
  const self = p.id === viewerId();

  if (editable) {
    const personTasks = staleItems().filter(i => i.person && i.person.id === p.id);
    if (personTasks.length) {
      const wrap = el('div', 'container');
      wrap.append(todoChecklist(personTasks));
      page.append(wrap);
    }
  }
  page.append(crumbsRow);

  const content = el('div', 'container detail-content');
  const headerCard = el('div', 'detail-card');
  const grid = el('div', 'detail-grid');
  const left = el('div');
  const wrap = el('div', 'photo-wrap' + (nagPhoto ? ' needs-update' : ''));
  const status = el('div', 'media-status');
  let onHeroPreview = null;
  const families = familiesOf(p);
  const family = families[0];
  const shared = familyPhoto(family);
  if (photos.length) {
    const img = el('img', 'detail-photo');
    img.src = photoUrl(photos[0]);
    img.alt = '';
    wrap.append(img);
    const updateCropBadge = photo => {
      wrap.querySelector('.photo-crop-badge')?.remove();
      if (cropped(photo)) {
        wrap.append(cropBadge());
      }
    };
    updateCropBadge(photos[0]);
    if (editable) {
      let current = photos[0];
      const menu = photoMenu(photos, () => current, editing, status);
      img.addEventListener('click', () => togglePhotoMenu(menu));
      wrap.append(menu);
      onHeroPreview = photo => {
        current = photo;
        updateCropBadge(photo);
      };
    } else {
      img.addEventListener('click', () => openPhotoLightbox(fullOf(photos[0])));
      onHeroPreview = updateCropBadge;
    }
  } else if (shared) {
    const img = el('img', 'detail-photo');
    img.src = photoUrl(shared);
    img.alt = '';
    img.addEventListener('click', () => openPhotoLightbox(photoUrl(shared)));
    wrap.append(img);
  } else {
    wrap.append(photoOrInitials(null, p.name_show, 'detail-photo detail-photo-empty'));
  }
  left.append(wrap);
  if (showPhotoEdit) {
    if (nagPhoto) {
      status.textContent = `Add ${self ? 'your' : `${firstName(p.name_show)}'s`} photo for the new year`;
    }
    wrap.append(uploadIcon('camera', 'Upload photo', 'image/*', file => uploadPhoto({person: p.id}, file, status)));
  } else if (nagPhoto) {
    left.append(el('div', 'media-status', `Update ${self ? 'your' : `${firstName(p.name_show)}'s`} photo for the new year`));
  }
  if (photos.length) {
    left.append(photoGrid(p, photos, editable, editing, wrap.querySelector('.detail-photo'), status, onHeroPreview));
  }
  if (editable) {
    left.append(status);
  }
  grid.append(left);

  const right = el('div');
  const topRow = el('div', 'detail-top');
  const role = baseRole(p);
  const roleRow = el('a', 'role-label role-label-' + role.toLowerCase(), role);
  roleRow.href = withFrom(role === 'Staff' ? '/staff' : '/people');
  topRow.append(roleRow);
  const topRight = el('div', 'detail-top-right');
  const classroom = classroomOf(p);
  if ((isStaff(p) || isStudent(p)) && classroom) {
    const classroomChip = el('a', 'tag-chip', classroom.name);
    classroomChip.href = withFrom(classroomPath(classroom));
    tintChip(classroomChip, classroom.color);
    topRight.append(classroomChip);
    const crew = groupById[p.crew];
    if (crew) {
      const crewChip = el('a', 'tag-chip', crew.name);
      crewChip.href = withFrom(classroomPath(classroom));
      tintChip(crewChip, paletteColor(crew.name));
      topRight.append(crewChip);
    }
  }
  if (editable) {
    const topActions = el('div', 'detail-top-actions');
    topActions.append(copyButton(personSummaryText(p, family), 'Copy all info'));
    topRight.append(topActions);
  }
  if (topRight.children.length) {
    topRow.append(topRight);
  }
  right.append(topRow);
  const nameHeader = el('h1', 'detail-name');
  nameHeader.append(el('span', '', p.name_show));
  if (p.pronunciation && !editing) {
    nameHeader.append(pronouncePill(pronunciationUrl(p), firstName(p.name_show)));
  }
  if (editing) {
    const pencil = editPencil('Edit preferred name');
    nameHeader.append(pencil);
    pencil.addEventListener('click', () => fieldEditor(nameHeader, pencil, {
      current: p.name_short_override || p.name_short || '',
      submit: (value, status) => saveCells(p.id, namesFor(p, value), status),
    }));
  }
  if (p.pronouns || editing) {
    const pronounSpan = el('span', 'detail-pronouns', p.pronouns ? formatPronouns(p.pronouns) : (editing ? 'pronouns' : ''));
    nameHeader.append(pronounSpan);
    if (editing) {
      const pronounPencil = editPencil('Edit pronouns');
      nameHeader.append(pronounPencil);
      pronounPencil.addEventListener('click', () => fieldEditor(pronounSpan, pronounPencil, {
        current: p.pronouns || '',
        allowHide: true,
        presets: [
          {label: 'she/her', value: 'she/her'},
          {label: 'he/him', value: 'he/him'},
          {label: 'they/them', value: 'they/them'},
        ],
        submit: (value, status) => saveCells(p.id, {pronouns: value}, status),
      }));
    }
  }
  right.append(nameHeader);
  const legal = legalNameLine(p);
  if (legal) {
    right.append(el('div', 'detail-sub', legal));
  }
  if (isStudent(p)) {
    const chain = gradeChain(p);
    if (chain) {
      right.append(el('div', 'detail-sub', chain));
    }
  } else if (isStaff(p) && p.job_title) {
    right.append(el('div', 'detail-sub', p.job_title));
  }
  const email = emailOf(p);
  if (email) {
    const emailValue = el('div', 'contact-value');
    emailValue.append(svg('mail'), el('span', '', email));
    right.append(contactRow(emailValue, [
      iconLink('mail', 'Email', 'mailto:' + email),
      copyButton(email),
    ]));
  }
  if (p.phone) {
    const phoneValue = el('div', 'contact-value');
    phoneValue.append(svg('phone'), el('span', '', p.phone));
    right.append(contactRow(phoneValue, [
      iconLink('chat', 'Text', 'sms:' + p.phone),
      iconLink('phone', 'Call', 'tel:' + p.phone),
      copyButton(p.phone),
    ]));
  }
  if (family && family.address) {
    const addressValue = el('div', 'contact-value');
    addressValue.append(svg('map'), el('span', '', family.address));
    right.append(contactRow(addressValue, [
      iconLink('map', 'Map', 'https://maps.google.com/?q=' + encodeURIComponent(family.address)),
      copyButton(family.address),
    ]));
  }
  if (editing) {
    right.append(el('div', 'pronounce-label', 'How do I pronounce this?'));
    if (p.pronunciation) {
      const audio = el('audio', 'pronounce-player');
      audio.controls = true;
      audio.preload = 'metadata';
      audio.src = pronunciationUrl(p);
      right.append(audio);
    }
    right.append(pronounceEditor({person: p.id}, p.id, Boolean(p.pronunciation)));
  }
  grid.append(right);
  headerCard.append(grid);
  content.append(headerCard);

  if (p.facts || showFactsEdit) {
    const aboutCard = el('div', 'detail-card');
    const header = el('h2', 'about-header', 'About Me');
    aboutCard.append(header);
    const needsFacts = factsNeedUpdate(p);
    const placeholder = needsFacts ? `Add ${self ? 'your' : `${firstName(p.name_show)}'s`} facts for the new year — click the pencil to get started.` : '';
    const textClass = 'about-text' + (editable && needsFacts ? ' needs-update' : '') + (!p.facts && placeholder ? ' placeholder-text' : '');
    const text = p.facts ? aboutMeText(textClass, p.facts) : el('div', textClass, placeholder);
    const status = el('div', 'media-status about-status');
    aboutCard.append(text);
    if (p.facts) {
      const when = monthYear(p.facts_updated);
      if (editable && needsFacts) {
        aboutCard.append(el('div', 'about-note about-note-stale',
          when ? `Posted ${when} — please refresh this for the new year.` : 'Please refresh this for the new year.'));
      } else if (!editable && when) {
        aboutCard.append(el('div', 'about-note', `Posted ${when}`));
      }
    }
    aboutCard.append(status);
    if (showFactsEdit) {
      const pencil = el('button', 'edit-icon inline');
      pencil.title = 'Edit';
      pencil.append(svg('edit'));
      header.append(pencil);
      pencil.addEventListener('click', () => {
        const editor = el('textarea', 'about-editor');
        editor.value = p.facts || '';
        const buttons = el('div', 'about-buttons');
        const save = el('button', 'media-button primary', 'Save');
        const cancel = el('button', 'media-button', 'Cancel');
        buttons.append(save, cancel);
        text.replaceWith(editor);
        editor.after(buttons);
        pencil.hidden = true;
        editor.focus();
        cancel.addEventListener('click', () => {
          buttons.remove();
          editor.replaceWith(text);
          pencil.hidden = false;
        });
        save.addEventListener('click', () => saveCells(p.id, {facts: editor.value.trim(), facts_updated: today()}, status));
      });
      if (focusFacts) {
        queueMicrotask(() => pencil.click());
      }
    }
    content.append(aboutCard);
  }

  page.append(content);

  if (families.length) {
    const wrap = el('div', 'container fcard-wrap' + (families.length > 1 ? ' fcard-wrap-multi' : ''));
    const row = el('div', 'fcard-columns');
    for (const f of families) {
      row.append(familyCard(p, f));
    }
    wrap.append(row);
    page.append(wrap);
  }
  return page;
}
