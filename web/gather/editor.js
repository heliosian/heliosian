import {el, button, svg, toast} from '/elements.js';
import {openModal} from '/modal.js';
import {field, text, textarea, whenPickers} from '/form.js';
import {navigate} from '/router.js';
import {themes, lookTheme, wrapped, sealMark, fontChoices} from '/themes.js';
import {createInvite, editInvite, asEvent, dayWords, hourWords, setDesignPicture, clearDesignPicture, uploadTheme, removeTheme, me, state} from './state.js';
import {offerUpdate, hostsRow} from '/invites.js';
import {fetchInvites as fetchGuestList} from '/guestlist.js';
import {miniEnvelope, sealedEnvelope, inviteCard, andList} from '/envelope.js';

function sidePages(panels) {
  const wrap = el('div', 'editor-pages');
  const nav = el('nav', 'editor-pages-nav');
  const bodies = [];
  const links = [];
  let active = 0;
  const show = i => {
    active = i;
    links.forEach((link, j) => link.classList.toggle('is-active', j === i));
    bodies.forEach((body, j) => {
      body.hidden = j !== i;
    });
    panels[i].onShow?.();
  };
  panels.forEach((panel, i) => {
    const link = el('button', 'editor-pages-link' + (panel.sub ? ' is-sub' : ''));
    link.type = 'button';
    if (panel.icon) {
      link.append(svg(panel.icon));
    }
    link.append(el('span', '', panel.label));
    if (panel.onToggle) {
      const chevron = el('span', 'editor-pages-chevron');
      chevron.setAttribute('role', 'button');
      chevron.setAttribute('aria-label', `Show or hide ${panel.label}’s pages`);
      chevron.append(svg('chevron-down'));
      chevron.addEventListener('click', ev => {
        ev.stopPropagation();
        link.classList.toggle('is-open', panel.onToggle());
      });
      link.classList.add('is-open');
      link.append(chevron);
    }
    link.addEventListener('click', () => show(i));
    nav.append(link);
    links.push(link);
    const body = el('div', 'editor-pages-body');
    body.append(...panel.fields);
    body.addEventListener('invalid', () => {
      if (active !== i) {
        show(i);
      }
    }, true);
    bodies.push(body);
  });
  const main = el('div', 'editor-pages-main');
  main.append(...bodies);
  wrap.append(nav, main);
  show(0);
  return {wrap, show, reveal: (i, on) => {
    links[i].hidden = !on;
  }};
}

function plainTheme() {
  return themes().find(t => t.key === 'plain' && !t.addedBy);
}

function usableThemes() {
  return themes().filter(t => !t.addedBy || t.addedBy === me().id);
}

function isPlain(id) {
  const plain = plainTheme();
  return Boolean(plain) && plain.id === id;
}

function defaultTheme() {
  const first = usableThemes().find(t => !isPlain(t.id)) || plainTheme();
  return first ? first.id : '';
}

function removableTheme(t) {
  return state.model.gatherAdmin && !t.addedBy && t.key !== 'plain';
}

function removeButton(t, onRemove) {
  const remove = el('span', 'theme-tile-remove');
  remove.setAttribute('role', 'button');
  remove.setAttribute('aria-label', `Delete ${t.name} for everyone`);
  remove.title = `Delete ${t.name} for everyone`;
  remove.append(svg('trash'));
  remove.addEventListener('click', ev => {
    ev.stopPropagation();
    if (!confirm(`Delete ${t.name} for everyone? Hosts won't be able to pick it any more. A theme an invitation uses can't be deleted.`)) {
      return;
    }
    removeTheme(t.id).then(() => onRemove(t.id)).catch(err => toast(err.message));
  });
  return remove;
}

function picker(label, drawTile, onPick, blurbs = true, lead = [], onRemove = null) {
  let chosen = '';
  const grid = el('div', 'theme-picker' + (blurbs ? '' : ' is-compact'));
  grid.setAttribute('role', 'radiogroup');
  grid.setAttribute('aria-label', label);
  const tiles = new Map();
  const mark = () => {
    for (const [id, tile] of tiles) {
      tile.classList.toggle('is-on', id === chosen);
      tile.setAttribute('aria-checked', String(id === chosen));
    }
  };
  const fill = () => {
    tiles.clear();
    grid.replaceChildren(...lead);
    for (const t of usableThemes().filter(x => !lead.length || !isPlain(x.id))) {
      const tile = el('button', 'theme-tile');
      tile.type = 'button';
      tile.setAttribute('role', 'radio');
      tile.append(drawTile(t), el('span', 'theme-name', t.name));
      if (blurbs) {
        tile.append(el('span', 'theme-blurb', t.addedBy ? 'Yours: ' + t.blurb : t.blurb));
      }
      if (onRemove && removableTheme(t)) {
        tile.append(removeButton(t, onRemove));
      }
      tile.addEventListener('click', () => onPick(t.id));
      grid.append(tile);
      tiles.set(t.id, tile);
    }
    mark();
  };
  fill();
  const wrap = el('div', 'field');
  wrap.append(el('span', '', label), grid);
  return {wrap, refresh: fill, set: id => {
    chosen = id;
    mark();
  }};
}

async function entryFiles(entry) {
  if (entry.isFile) {
    return [await new Promise((ok, fail) => entry.file(ok, fail))];
  }
  const reader = entry.createReader();
  const out = [];
  for (;;) {
    const batch = await new Promise((ok, fail) => reader.readEntries(ok, fail));
    if (!batch.length) {
      return out;
    }
    for (const e of batch) {
      out.push(...await entryFiles(e));
    }
  }
}

function filePicker(label, folder, add) {
  const input = el('input');
  input.type = 'file';
  input.multiple = true;
  input.webkitdirectory = folder;
  input.hidden = true;
  input.addEventListener('change', () => {
    add([...input.files]);
    input.value = '';
  });
  const pick = el('button', 'link-button', label);
  pick.type = 'button';
  pick.addEventListener('click', () => input.click());
  return [pick, input];
}

function themeUpload(onUploaded) {
  let files = [];
  const zone = el('div', 'theme-drop');
  const words = el('p', 'theme-drop-words', 'Drop your theme here: its folder, a zip of it, or theme.json with its pictures.');
  const list = el('ul', 'theme-drop-list');
  const add = more => {
    files = [...files, ...more.filter(f => !f.name.startsWith('.') && !files.some(x => x.name === f.name && x.size === f.size))];
    list.replaceChildren(...files.map(f => el('li', '', f.name)));
    zone.classList.toggle('has-files', files.length > 0);
  };
  const clear = el('button', 'link-button', 'Start over');
  clear.type = 'button';
  clear.addEventListener('click', () => {
    files = [];
    add([]);
  });
  const picks = el('p', 'theme-drop-picks');
  picks.append('Or choose ', ...filePicker('files', false, add), ' or ', ...filePicker('a folder', true, add), '. ', clear);
  zone.append(svg('upload'), words, picks, list);
  zone.addEventListener('dragover', ev => {
    ev.preventDefault();
    zone.classList.add('is-over');
  });
  zone.addEventListener('dragleave', () => zone.classList.remove('is-over'));
  zone.addEventListener('drop', async ev => {
    ev.preventDefault();
    zone.classList.remove('is-over');
    const entries = [...ev.dataTransfer.items].map(i => i.webkitGetAsEntry()).filter(Boolean);
    const found = [];
    for (const entry of entries) {
      found.push(...await entryFiles(entry));
    }
    add(found);
  });
  const kit = el('p', 'theme-kit-note');
  const link = el('a', '', 'Download the theme kit');
  link.href = '/open/theme-kit.zip';
  kit.append(link, ': instructions, examples and a preview page that checks a theme before you upload it. Hand it to an AI to design one. Uploading a theme again with the same key replaces it.');
  openModal('Upload a theme', [kit, zone], {
    saveLabel: 'Upload',
    submit: () => {
      if (!files.length) {
        throw new Error('Drop your theme in first.');
      }
      return uploadTheme(files);
    },
    afterSave: onUploaded,
  });
}

function cardPreview(words) {
  const box = el('div', 'theme-card-preview');
  const card = inviteCard(words);
  box.append(card);
  new ResizeObserver(() => {
    if (!card.offsetWidth) {
      return;
    }
    const scale = box.clientWidth / card.offsetWidth;
    card.style.transform = `scale(${scale})`;
    box.style.height = `${card.offsetHeight * scale}px`;
  }).observe(box);
  return box;
}

function hostsField(inv, hostedBy) {
  const wrap = el('div', 'field editor-hosts');
  if (!inv) {
    wrap.append(el('span', '', 'Hosts'), el('small', '', 'You host it. Add co-hosts once it’s created.'));
    return wrap;
  }
  const e = asEvent(inv);
  const fill = () => fetchGuestList(e).then(view => {
    hostedBy.names(andList(view.hosts.map(h => h.name || h.email).filter(Boolean)));
    wrap.replaceChildren(hostsRow(e, view, fill));
  }, err => {
    wrap.replaceChildren(el('p', 'save-status error', 'Couldn’t load the hosts: ' + err.message));
  });
  fill();
  return wrap;
}

function hostedByField(current) {
  const input = text(current, {placeholder: 'The host names'});
  let names = '';
  const wrap = field('Hosted by', input, 'What the card says after “Hosted by”. It starts as the hosts’ names; write anything in their place.');
  return {
    wrap,
    names: list => {
      if (!input.value || input.value === names) {
        input.value = list;
      }
      names = list;
      input.placeholder = list;
    },
    value: () => {
      const v = input.value.trim();
      return v === names ? '' : v;
    },
  };
}

export function firstHex(value) {
  const m = String(value).match(/#[0-9a-fA-F]{6}/);
  return m ? m[0].toLowerCase() : '#ffffff';
}

function colorField(label, path, changes, template, repaint) {
  const [section, key] = path.split('.');
  const input = el('input', 'color-swatch');
  input.type = 'color';
  const reset = button('Use the design’s', '', 'link-button', () => {
    delete changes[section][key];
    sync();
    repaint();
  });
  const sync = () => {
    const own = changes[section][key];
    input.value = own || firstHex(template()[section][key]);
    reset.hidden = !own;
  };
  input.addEventListener('input', () => {
    changes[section][key] = input.value;
    reset.hidden = false;
    repaint();
  });
  const row = el('div', 'color-row');
  row.append(input, reset);
  const wrap = field(label, row);
  return {wrap, sync};
}

function fontField(label, path, changes, repaint) {
  const [section, key] = path.split('.');
  const select = el('select');
  select.append(new Option('The design’s own', ''));
  for (const f of fontChoices()) {
    const option = new Option(f.name, f.stack);
    option.style.fontFamily = f.stack;
    select.append(option);
  }
  select.value = changes[section][key] || '';
  select.addEventListener('change', () => {
    if (select.value) {
      changes[section][key] = select.value;
    } else {
      delete changes[section][key];
    }
    repaint();
  });
  return field(label, select);
}

export function pictureInput(label, current, upload, clear, changed) {
  const wrap = el('div', 'field picture-field');
  const status = el('span', 'save-status');
  let url = current;
  const paint = () => {
    const row = el('div', 'picture-row');
    if (url) {
      const img = el('img', 'picture-thumb');
      img.src = url;
      img.alt = '';
      row.append(img, button('Remove', 'trash', 'link-button', async () => {
        status.textContent = 'Removing…';
        try {
          await clear();
          url = '';
          status.textContent = '';
          paint();
          changed(url);
        } catch (err) {
          status.textContent = err.message;
        }
      }));
    }
    const file = el('input');
    file.type = 'file';
    file.accept = 'image/*';
    file.hidden = true;
    file.addEventListener('change', async () => {
      const picked = file.files[0];
      if (!picked) {
        return;
      }
      status.textContent = 'Uploading…';
      try {
        await upload(picked);
        url = URL.createObjectURL(picked);
        status.textContent = '';
        paint();
        changed(url);
      } catch (err) {
        status.textContent = err.message;
      }
    });
    row.append(file, button(url ? 'Change picture' : 'Upload a picture', 'upload', 'button button-secondary', () => file.click()));
    wrap.replaceChildren(el('span', '', label), row, status);
  };
  paint();
  return wrap;
}

function pictureField(label, inv, part, pictures, repaint) {
  if (!inv) {
    const wrap = el('div', 'field picture-field');
    wrap.append(el('span', '', label), el('small', '', 'Create the invitation first, then add pictures here.'));
    return {wrap};
  }
  const wrap = pictureInput(label, pictures[part], file => setDesignPicture(inv, part, file), () => clearDesignPicture(inv, part), url => {
    pictures[part] = url;
    repaint();
  });
  return {wrap};
}

function customizePage(inv, changes, pictures, template, lookNow, sample, repaintAll) {
  const preview = el('div', 'customize-preview');
  const repaint = () => {
    preview.replaceChildren(cardPreview({...sample(), look: lookNow()}));
    words.hidden = !pictures.card;
    repaintAll();
  };
  const words = el('label', 'check-row');
  const wordsBox = el('input');
  wordsBox.type = 'checkbox';
  wordsBox.checked = changes.card.words === false;
  wordsBox.addEventListener('change', () => {
    if (wordsBox.checked) {
      changes.card.words = false;
    } else {
      delete changes.card.words;
    }
    repaint();
  });
  words.append(wordsBox, el('span', '', 'My picture already has the words on it'));
  const colors = [
    colorField('Card', 'card.background', changes, template, repaint),
    colorField('Writing', 'card.ink', changes, template, repaint),
    colorField('Accent', 'card.accent', changes, template, repaint),
    colorField('Page behind the card', 'page.backdrop', changes, template, repaint),
  ];
  const card = pictureField('Card picture', inv, 'card', pictures, repaint);
  const backdrop = pictureField('Page background picture', inv, 'backdrop', pictures, repaint);
  const colorGrid = el('div', 'color-grid');
  colorGrid.append(...colors.slice(0, 3).map(c => c.wrap));
  const fonts = el('div', 'color-grid');
  fonts.append(fontField('Title font', 'fonts.display', changes, repaint), fontField('Text font', 'fonts.body', changes, repaint));
  return {
    preview,
    tabs: [
      {label: 'Card', fields: [card.wrap, words]},
      {label: 'Background', fields: [backdrop.wrap, colors[3].wrap]},
      {label: 'Colors', fields: [colorGrid]},
      {label: 'Fonts', fields: [fonts]},
    ],
    show: () => {
      for (const c of colors) {
        c.sync();
      }
      repaint();
    },
  };
}

function sealSwatch(t, name) {
  return wrapped(el('span', 'seal-swatch', sealMark({theme: t.id}, name)), {theme: t.id});
}

function linerSwatch(t) {
  return wrapped(el('span', 'liner-swatch'), {theme: t.id});
}

export function openEditor(inv) {
  if (!inv && !defaultTheme()) {
    toast('Gather has no themes yet, so there is nothing to draw an invitation in. A Gather admin needs to add one.');
    return;
  }
  const look = inv ? inv.look : {theme: '', envelope: '', seal: '', liner: '', design: {}};
  const chosen = {theme: look.theme || defaultTheme(), envelope: look.envelope, seal: look.seal, liner: look.liner};
  const changes = {card: {}, page: {}, fonts: {}};
  for (const section of Object.keys(changes)) {
    Object.assign(changes[section], look.design?.[section] || {});
  }
  const pictures = {card: inv ? look.cardPicture : '', backdrop: inv ? look.backdrop : ''};
  const name = text(inv ? inv.name : '', {required: true, placeholder: 'Maya’s 8th Birthday'});
  const subtitle = text(inv ? inv.subtitle : '', {placeholder: 'An afternoon of bouncing and cake'});
  const title = () => name.value.trim() || 'Your invitation';
  const designOf = () => Object.fromEntries(Object.entries(changes).filter(([, v]) => Object.keys(v).length));
  const lookNow = () => ({...chosen, design: designOf(), cardPicture: pictures.card, backdrop: pictures.backdrop});
  const template = () => lookTheme({theme: chosen.theme});
  const hostedBy = hostedByField(inv ? inv.hostedBy : '');
  const sample = () => ({day: inv ? dayWords(inv) : 'Saturday, December 12', hours: inv ? hourWords(inv) : '2:00 – 4:00 PM', location: '', description: '', hosts: [], hostedBy: ''});
  const fallback = {
    envelope: () => chosen.theme,
    seal: () => chosen.envelope || chosen.theme,
    liner: () => chosen.envelope || chosen.theme,
  };
  let envelopeStage = null;
  const pickers = {};
  const marks = () => {
    pickers.theme.set(chosen.theme);
    own.classList.toggle('is-on', isPlain(chosen.theme));
    for (const part of ['envelope', 'seal', 'liner']) {
      pickers[part].set(chosen[part] || fallback[part]());
    }
  };
  const repaintEnvelope = () => {
    const stage = sealedEnvelope(lookNow(), title(), 'How your guests will see it', () => {});
    stage.inert = true;
    envelopeStage.replaceChildren(stage);
  };
  const pickPart = part => key => {
    chosen[part] = key === fallback[part]() ? '' : key;
    marks();
    repaintEnvelope();
  };
  let themeChosen = Boolean(inv && look.theme);
  const own = el('button', 'theme-tile build-own');
  own.type = 'button';
  const ownArt = el('div', 'build-own-art');
  ownArt.append(svg('upload'), el('span', '', 'Your picture'));
  own.append(ownArt, el('span', 'theme-name', 'Create Your Own'), el('span', 'theme-blurb', 'Upload your own card and background, then pick colors and fonts'));
  const upload = el('button', 'theme-tile build-own');
  upload.type = 'button';
  const uploadArt = el('div', 'build-own-art');
  uploadArt.append(svg('doc'), el('span', '', 'A theme file'));
  upload.append(uploadArt, el('span', 'theme-name', 'Upload a theme'), el('span', 'theme-blurb', 'A whole design of your own: card, envelope, seal and liner'));
  const pick = id => {
    chosen.theme = id;
    themeChosen = true;
    marks();
    paintTheme();
  };
  pickers.theme = picker('Theme', t => cardPreview({...sample(), look: {theme: t.id}, name: title()}), pick, true, plainTheme() ? [own, upload] : [upload], id => {
    for (const part of ['theme', 'envelope', 'seal', 'liner']) {
      if (chosen[part] === id) {
        chosen[part] = part === 'theme' ? defaultTheme() : '';
      }
    }
    for (const p of Object.values(pickers)) {
      p.refresh();
    }
    marks();
  });
  own.addEventListener('click', () => pick(plainTheme().id));
  upload.addEventListener('click', () => themeUpload(id => {
    for (const p of Object.values(pickers)) {
      p.refresh();
    }
    pick(id);
  }));
  pickers.envelope = picker('Envelope', t => miniEnvelope({theme: t.id}, title()), pickPart('envelope'));
  pickers.seal = picker('Seal', t => sealSwatch(t, title()), pickPart('seal'), false);
  pickers.liner = picker('Liner', t => linerSwatch(t), pickPart('liner'), false);
  envelopeStage = el('div', 'editor-envelope');
  const customize = customizePage(inv, changes, pictures, template, lookNow, () => ({...sample(), name: title()}), () => {});
  const starts = whenPickers('Starts', inv ? inv.start : '', null, 'Clear');
  const ends = whenPickers('Ends', inv ? inv.end : '', null, 'Clear');
  const location = text(inv ? inv.location : '', {placeholder: 'Where it happens'});
  const summary = textarea(inv ? inv.description : '', 4);
  summary.placeholder = 'A few words to your guests';
  const showEnds = Boolean(inv && inv.end && inv.end.slice(0, 10) !== inv.start.slice(0, 10));
  ends.wrap.hidden = !showEnds;
  const until = el('input');
  until.type = 'time';
  until.value = inv && !showEnds && inv.end.length > 10 && inv.end !== inv.start ? inv.end.slice(11, 16) : '';
  const untilWrap = el('div', 'field-when-block field-when-until');
  const untilHead = el('div', 'field-when-head');
  untilHead.append(el('span', 'field-when-label', 'Ends at'));
  untilWrap.append(untilHead, until);
  untilWrap.hidden = showEnds;
  const more = button('Ends on a different day', 'plus', 'link-button field-when-more', () => {
    ends.wrap.hidden = false;
    untilWrap.hidden = true;
    more.hidden = true;
    ends.focus();
  });
  more.hidden = showEnds;
  const body = () => {
    const start = starts.value();
    const end = !ends.wrap.hidden ? ends.value() : until.value && start ? `${start.slice(0, 10)} ${until.value}` : '';
    if (end && start && end < start) {
      throw new Error('The end is before the start.');
    }
    return {
      name: name.value.trim(), subtitle: subtitle.value.trim(), start, end, location: location.value.trim(), description: summary.value.trim(),
      theme: chosen.theme, envelope: chosen.envelope, seal: chosen.seal, liner: chosen.liner, design: designOf(), hostedBy: hostedBy.value(),
    };
  };
  let changed = [];
  const when = el('div', 'field-when');
  when.append(starts.wrap, untilWrap, ends.wrap, more);
  const themeArea = el('div', 'theme-area');
  const subs = customize.tabs.map(tab => {
    const slot = el('div', 'customize-slot');
    const grid = el('div', 'customize');
    const fields = el('div', 'editor-pages-body');
    fields.append(...tab.fields);
    grid.append(fields, slot);
    return {label: tab.label, sub: true, fields: [grid], onShow: () => {
      slot.append(customize.preview);
      customize.show();
    }};
  });
  subs.push({label: 'Envelope', sub: true, fields: [envelopeStage, pickers.envelope.wrap, pickers.seal.wrap, pickers.liner.wrap], onShow: repaintEnvelope});
  let pages = null;
  let subsOpen = true;
  const revealSubs = () => {
    subs.forEach((_, j) => pages.reveal(3 + j, themeChosen && subsOpen));
  };
  function paintTheme() {
    revealSubs();
    if (!themeChosen) {
      themeArea.replaceChildren(pickers.theme.wrap);
      return;
    }
    const t = themes().find(x => x.id === chosen.theme);
    const head = el('div', 'theme-head');
    const words = el('div', 'theme-head-words');
    words.append(el('span', 'theme-head-label', 'Theme'), el('span', 'theme-head-name', isPlain(chosen.theme) ? 'Your own' : t.name));
    head.append(words, button('Change theme', 'edit', 'button button-secondary', () => {
      themeChosen = false;
      paintTheme();
    }));
    const preview = el('div', 'theme-current');
    preview.append(cardPreview({...sample(), look: lookNow(), name: title()}));
    themeArea.replaceChildren(head, el('p', 'theme-hint', 'Change its card, background, colors, fonts and envelope from the pages under Theme.'), preview);
  }
  pages = sidePages([
    {label: 'Details', icon: 'list', fields: [field('Title', name, '', true), field('Subtitle', subtitle, 'A line under the title on the card. Leave it blank for none.'), when, field('Where', location), field('Summary', summary)]},
    {label: 'Essentials', icon: 'groups', fields: [hostsField(inv, hostedBy), hostedBy.wrap]},
    {label: 'Theme', icon: 'doc', fields: [themeArea], onShow: paintTheme, onToggle: () => {
      subsOpen = !subsOpen;
      revealSubs();
      return subsOpen;
    }},
    ...subs,
  ]);
  revealSubs();
  marks();
  openModal(inv ? 'Edit invitation' : 'New invitation', [pages.wrap], {
    wide: true,
    saveLabel: inv ? 'Save' : 'Create invitation',
    submit: async () => {
      const b = body();
      if (!inv) {
        return createInvite(b);
      }
      changed = [['title', b.name !== inv.name], ['start', b.start !== inv.start], ['end', (b.end || b.start) !== inv.end], ['location', b.location !== inv.location]].filter(([, c]) => c).map(([k]) => k);
      await editInvite(inv, b);
      return '';
    },
    afterSave: id => {
      if (!inv && id) {
        navigate(`/g/${id}`);
        return;
      }
      if (inv && changed.length) {
        offerUpdate(asEvent(inv), changed);
      }
    },
  });
}
