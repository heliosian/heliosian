import {load} from '../app.js';
import {colors} from '../state.js';
import {el, svg, withFrom, slugify, thumbUrl, firstName, iconButton, copyButton, pronouncePill, contactRow, aboutMeText, paletteColor} from '../dom.js';
import {familiesOf} from '../families.js';
import {personByKey, baseRole, gradeChain, photoOrInitials, formatPronouns} from '../people.js';
import {photoNeedsUpdate, factsNeedUpdate, staleItems, todoChecklist, monthYear} from '../stale.js';
import {canEditPerson, submitField, editPencil, fieldEditor, uploadIcon, pronounceEditor} from '../edit.js';
import {openPhotoLightbox, cropBadge, photoMenu, togglePhotoMenu, photoGrid} from '../photos.js';
import {fromCrumbs, breadcrumbs} from '../crumbs.js';
import {resetMain, finishRender} from '../chrome.js';
import {familyCard} from './family.js';

// Light-tint pairing (pale background, solid text) matching role-label and
// the family-card grade chips, instead of .tag-chip's flat neutral gray -
// so a classroom/crew chip reads with that classroom's own admin color.
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

export function renderPersonDetail(email) {
  const main = resetMain();
  const p = personByKey(email);
  if (!p) {
    main.append(el('div', 'empty', 'Not found.'));
    return;
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
  // Held rather than appended immediately - on mobile the crumb trail itself is
  // hidden (see .crumbs), leaving just the tag button, which reads better below
  // the alert cards and right above the profile than sandwiched between them.
  const crumbsRow = breadcrumbs([...origin, [p.fullName, null]], p.email);

  const editable = canEditPerson(p.email);
  const editing = editable && personEdit === p.email;
  // The "update for the new year" nag (dashed outline + reminder text) is student-only,
  // same as elsewhere. The camera icon is now only for bootstrapping someone with no
  // photo at all - once they have at least one, the photo grid below (with its own
  // "+" tile) is the only way to add more, so there's exactly one way to do it.
  const nagPhoto = editable && photoNeedsUpdate(p);
  const showPhotoEdit = editable && (p.photos || []).length === 0;
  const showFactsEdit = editing || (editable && factsNeedUpdate(p));
  const self = p.email === document.body.dataset.userEmail;

  if (editable) {
    const personTasks = staleItems().filter(i => i.person && i.person.email === p.email);
    if (personTasks.length) {
      const wrap = el('div', 'container');
      wrap.append(todoChecklist(personTasks));
      main.append(wrap);
    }
  }
  main.append(crumbsRow);

  const content = el('div', 'container detail-content');
  const headerCard = el('div', 'detail-card');
  const grid = el('div', 'detail-grid');
  const left = el('div');
  const wrap = el('div', 'photo-wrap' + (nagPhoto ? ' needs-update' : ''));
  // Shared between the hero's own photo menu and photoGrid below, so an action
  // from either place reports success/error in the same spot.
  const status = el('div', 'media-status');
  // Set by photoGrid below, if rendered, so tapping a grid tile to preview a
  // different photo in the hero also redirects the hero's own menu to act on
  // that photo instead of always the primary one - see the comment on
  // photoMenu's getPhoto param.
  let onHeroPreview = null;
  const families = familiesOf(p);
  const family = families[0];
  if (p.photoUrl) {
    // Person.Photos is `omitempty` in the JSON, so p.photos is undefined - not []
    // - whenever nobody has ever uploaded a photo for this person; every other
    // read of it already guards for that except the two below.
    const photos = p.photos || [];
    const img = el('img', 'detail-photo');
    img.src = p.photoUrl;
    img.alt = '';
    wrap.append(img);
    const initialHeroPhoto = photos.length ? photos[0] : {name: '', url: p.photoUrl, originalUrl: p.photoUrl};
    // Reflects whichever photo the hero is currently showing, same as
    // photoMenu's getPhoto - a preview swap (photoGrid's previewPhoto) can put a
    // different photo on screen than the one the page rendered with.
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
    // No uploaded photo of their own: fall back to the family photo rather than a
    // colored-initials placeholder, since that's the more recognizable default for
    // a parent (or a kid) whose own photo hasn't been added yet.
    const img = el('img', 'detail-photo');
    img.src = thumbUrl(family.photoUrl);
    img.alt = '';
    img.addEventListener('click', () => openPhotoLightbox(family.photoUrl));
    wrap.append(img);
  } else {
    // No uploaded photo, own or family's: fall back to the same colored-initials
    // shape the directory grid uses instead of an empty gray box, so a profile
    // never looks broken.
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
  // Staff has its own dedicated page; Student and Parent don't (Directory's
  // role chips filter it in place instead of linking anywhere), so both land
  // on Directory - still somewhere real and relevant, just not pre-filtered.
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
      // Crews have no admin-configured color (unlike grades/classrooms), so
      // this falls back to the same per-name hash color tags/departments use.
      tintChip(crewChip, paletteColor(p.crew));
      topRight.append(crewChip);
    }
  }
  if (editable) {
    const topActions = el('div', 'detail-top-actions');
    const toggle = editing
      ? el('button', 'media-button edit-toggle', 'Done')
      : iconButton('pencil', 'Edit info', () => {
        personEdit = p.email;
        renderPersonDetail(email);
        finishRender();
      });
    if (editing) {
      toggle.addEventListener('click', () => {
        personEdit = null;
        renderPersonDetail(email);
        finishRender();
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
      iconButton('mail', 'Email', 'mailto:' + p.email),
      copyButton(p.email),
    ]));
  }
  if (p.phone) {
    const phoneValue = el('div', 'contact-value');
    phoneValue.append(svg('phone'), el('span', '', p.phone));
    right.append(contactRow(phoneValue, [
      iconButton('message', 'Text', 'sms:' + p.phone),
      iconButton('phone', 'Call', 'tel:' + p.phone),
      copyButton(p.phone),
    ]));
  }
  if (family && family.address) {
    const addressValue = el('div', 'contact-value');
    addressValue.append(svg('map'), el('span', '', family.address));
    right.append(contactRow(addressValue, [
      iconButton('map', 'Map', 'https://maps.google.com/?q=' + encodeURIComponent(family.address)),
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
      pencil.append(svg('pencil'));
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
          const res = await fetch('/api/directory/facts', {method: 'POST', body: form});
          if (!res.ok) {
            status.classList.add('error');
            status.textContent = await res.text();
            return;
          }
          await load();
        });
      });
      if (focusFacts) {
        pencil.click();
      }
    }
    content.append(aboutCard);
  }

  main.append(content);

  // One identical card per family - a kid in two households gets both side by
  // side (stacking on narrow screens) rather than one buried below the other,
  // so it reads as "these are the two households" instead of a repeat.
  if (families.length) {
    const wrap = el('div', 'container fcard-wrap' + (families.length > 1 ? ' fcard-wrap-multi' : ''));
    const row = el('div', 'fcard-columns');
    for (const f of families) {
      row.append(familyCard(p, f));
    }
    wrap.append(row);
    main.append(wrap);
  }
}
