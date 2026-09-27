import {load, render} from '/router.js';
import {api} from '/api.js';
import {colors} from '../state.js';
import {withFrom, slugify, thumbUrl, firstName, copyButton, pronouncePill, contactRow, aboutMeText, paletteColor} from '../dom.js';
import {el, svg, iconButton, iconLink} from '/elements.js';
import {familiesOf} from '../families.js';
import {personByKey, baseRole, gradeChain, photoOrInitials, formatPronouns} from '../people.js';
import {photoNeedsUpdate, factsNeedUpdate, staleItems, todoChecklist, monthYear} from '../stale.js';
import {canEditPerson, submitField, editPencil, fieldEditor, uploadIcon, pronounceEditor} from '../edit.js';
import {openPhotoLightbox, cropBadge, photoMenu, togglePhotoMenu, photoGrid} from '../photos.js';
import {fromCrumbs, breadcrumbs} from '../crumbs.js';
import {familyCard} from './family.js';

function tintChip(chip, color) {
  if (color) {
    chip.style.background = `color-mix(in srgb, ${color} 20%, white)`;
    chip.style.color = color;
  }
}

function personSummaryText(p, family) {
  const lines = [p.fullName];
  lines.push(p.pronouns ? `${baseRole(p)} · ${formatPronouns(p.pronouns)}` : baseRole(p));
  lines.push('');
  if (p.phone) {
    lines.push('Phone: ' + p.phone);
  }
  if (!p.emailMasked) {
    lines.push('Email: ' + p.email);
  }
  if (family && family.address) {
    lines.push('Address: ' + family.address);
  }
  if (p.facts) {
    lines.push('', 'About: ' + p.facts);
  }
  return lines.join('\n');
}

function displayNameLine(p) {
  if (!p.legalName || p.legalName === p.fullName) {
    return null;
  }
  return p.legalName;
}

let personEdit = null;

export function personPage(email) {
  const page = document.createDocumentFragment();
  const p = personByKey(email);
  if (!p) {
    page.append(el('div', 'empty', 'Not found.'));
    return page;
  }
  const params = new URLSearchParams(location.search);
  const focusFacts = params.get('focus') === 'facts';
  if (params.get('edit') === '1') {
    params.delete('edit');
    params.delete('focus');
    const query = params.toString();
    history.replaceState(null, '', location.pathname + (query ? '?' + query : ''));
    personEdit = email;
  }
  const origin = fromCrumbs() || [['People', '/people']];
  const crumbsRow = breadcrumbs([...origin, [p.fullName, null]], p.email);

  const editable = canEditPerson(p.email);
  const editing = editable && personEdit === p.email;
  const nagPhoto = editable && photoNeedsUpdate(p);
  const showPhotoEdit = editable && (p.photos || []).length === 0;
  const showFactsEdit = editing || (editable && factsNeedUpdate(p));
  const self = p.email === document.body.dataset.userEmail;

  if (editable) {
    const personTasks = staleItems().filter(i => i.person && i.person.email === p.email);
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
  if (p.photoUrl) {
    const photos = p.photos || [];
    const img = el('img', 'detail-photo');
    img.src = p.photoUrl;
    img.alt = '';
    wrap.append(img);
    const initialHeroPhoto = photos.length ? photos[0] : {name: '', url: p.photoUrl, originalUrl: p.photoUrl};
    const updateCropBadge = photo => {
      wrap.querySelector('.photo-crop-badge')?.remove();
      if (photo.url !== photo.originalUrl) {
        wrap.append(cropBadge());
      }
    };
    updateCropBadge(initialHeroPhoto);
    if (editable) {
      let currentHeroPhoto = initialHeroPhoto;
      const menu = photoMenu(p, () => currentHeroPhoto, editing, status);
      img.addEventListener('click', () => togglePhotoMenu(menu));
      wrap.append(menu);
      onHeroPreview = photo => {
        currentHeroPhoto = photo;
        updateCropBadge(photo);
      };
    } else {
      img.addEventListener('click', () => openPhotoLightbox(photos.length ? photos[0].originalUrl : p.photoUrl));
      onHeroPreview = updateCropBadge;
    }
  } else if (family && family.photoUrl) {
    const img = el('img', 'detail-photo');
    img.src = thumbUrl(family.photoUrl);
    img.alt = '';
    img.addEventListener('click', () => openPhotoLightbox(family.photoUrl));
    wrap.append(img);
  } else {
    wrap.append(photoOrInitials(null, p.fullName, 'detail-photo detail-photo-empty'));
  }
  left.append(wrap);
  if (showPhotoEdit) {
    if (nagPhoto) {
      status.textContent = `Add ${self ? 'your' : `${firstName(p.fullName)}'s`} photo for the new year`;
    }
    wrap.append(uploadIcon('camera', 'Upload photo', 'image/*', 'person', p.email, 'photo', status));
  } else if (nagPhoto) {
    left.append(el('div', 'media-status', `Update ${self ? 'your' : `${firstName(p.fullName)}'s`} photo for the new year`));
  }
  if ((p.photos || []).length >= 1) {
    left.append(photoGrid(p, editable, editing, wrap.querySelector('.detail-photo'), status, onHeroPreview));
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
  if (p.isStaff || p.isStudent) {
    if (p.classroom) {
      const classroomChip = el('a', 'tag-chip', p.classroom);
      classroomChip.href = withFrom('/classrooms/' + slugify(p.classroom));
      tintChip(classroomChip, colors.classrooms[p.classroom]);
      topRight.append(classroomChip);
    }
    if (p.crew) {
      const crewChip = el('a', 'tag-chip', p.crew);
      crewChip.href = withFrom('/classrooms/' + slugify(p.classroom));
      tintChip(crewChip, paletteColor(p.crew));
      topRight.append(crewChip);
    }
  }
  if (editable) {
    const topActions = el('div', 'detail-top-actions');
    const toggle = editing
      ? el('button', 'media-button edit-toggle', 'Done')
      : iconButton('edit', 'Edit info', '', () => {
        personEdit = p.email;
        render();
      });
    if (editing) {
      toggle.addEventListener('click', () => {
        personEdit = null;
        render();
      });
    }
    topActions.append(toggle);
    topActions.append(copyButton(personSummaryText(p, family), 'Copy all info'));
    topRight.append(topActions);
  }
  if (topRight.children.length) {
    topRow.append(topRight);
  }
  right.append(topRow);
  const nameHeader = el('h1', 'detail-name');
  nameHeader.append(el('span', '', p.fullName));
  if (p.pronunciationUrl && !editing) {
    nameHeader.append(pronouncePill(p.pronunciationUrl, firstName(p.fullName)));
  }
  if (editing) {
    const pencil = editPencil('Edit preferred name');
    nameHeader.append(pencil);
    pencil.addEventListener('click', () => fieldEditor(nameHeader, pencil, {
      current: p.preferredName || '',
      submit: (value, status) => submitField(p.email, 'preferred-name', value, status),
    }));
  }
  if (p.pronouns || editing) {
    const pronounSpan = el('span', 'detail-pronouns', p.pronouns ? p.pronouns.toLowerCase().split('/').join(' / ') : (editing ? 'pronouns' : ''));
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
        submit: (value, status) => submitField(p.email, 'pronouns', value, status),
      }));
    }
  }
  right.append(nameHeader);
  const nickname = displayNameLine(p);
  if (nickname) {
    right.append(el('div', 'detail-sub', nickname));
  }
  if (p.isStudent) {
    const chain = gradeChain(p);
    if (chain) {
      right.append(el('div', 'detail-sub', chain));
    }
  } else if (p.isStaff && p.jobTitle) {
    right.append(el('div', 'detail-sub', p.jobTitle));
  }
  if (!p.emailMasked) {
    const emailValue = el('div', 'contact-value');
    emailValue.append(svg('mail'), el('span', '', p.email));
    right.append(contactRow(emailValue, [
      iconLink('mail', 'Email', 'mailto:' + p.email),
      copyButton(p.email),
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
    if (p.pronunciationUrl) {
      const audio = el('audio', 'pronounce-player');
      audio.controls = true;
      audio.preload = 'metadata';
      audio.src = p.pronunciationUrl;
      right.append(audio);
    }
    right.append(pronounceEditor('person', p.email, !!p.hasOwnPronunciation));
  }
  grid.append(right);
  headerCard.append(grid);
  content.append(headerCard);

  if (p.facts || showFactsEdit) {
    const aboutCard = el('div', 'detail-card');
    const header = el('h2', 'about-header', 'About Me');
    aboutCard.append(header);
    const needsFacts = factsNeedUpdate(p);
    const placeholder = needsFacts ? `Add ${self ? 'your' : `${firstName(p.fullName)}'s`} facts for the new year — click the pencil to get started.` : '';
    const textClass = 'about-text' + (editable && needsFacts ? ' needs-update' : '') + (!p.facts && placeholder ? ' placeholder-text' : '');
    const text = p.facts ? aboutMeText(textClass, p.facts) : el('div', textClass, placeholder);
    const status = el('div', 'media-status about-status');
    aboutCard.append(text);
    if (p.facts) {
      const when = monthYear(p.factsUpdated);
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
        save.addEventListener('click', async () => {
          status.classList.remove('error');
          status.textContent = 'Saving…';
          const form = new FormData();
          form.append('key', p.email);
          form.append('facts', editor.value);
          try {
            await api('POST', '/api/directory/facts', form);
          } catch (err) {
            status.classList.add('error');
            status.textContent = err.message;
            return;
          }
          await load();
        });
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
