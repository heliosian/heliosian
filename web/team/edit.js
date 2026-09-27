import {state, me, isAdmin, longDate, years, allYears, activityPath, activity, parentOf, canAdd, ADDING, category as categoryOf, descendants, rootOf, eventCategories, headingChoices, UNCATEGORIZED, isFamily} from './state.js';

function* allNodes() {
  for (const root of state.model.activities) {
    yield root;
    yield* descendants(root);
  }
}
import {el, svg, toast, button, thumb, whenEditor} from './dom.js';
import {imageTools} from '/images.js';
import {createPersonPicker} from '/picker.js';
import {whoLink} from '/toolbar.js';
import {api} from '/api.js';
import {openModal, closeModal} from '/modal.js';
import {field, text, textarea, select, checkbox, segmented} from '/form.js';
import {tabbedFields} from '/tabs.js';

export const {uploadAndSave, imageSearchOn, openImageSearch, imagePicker} = imageTools('/api/team', {state, toast});

export async function reload() {
  const {load} = await import('./app.js');
  await load();
}

export async function saveActivity(body) {
  const before = body.id && activity(body.id) ? activityPath(activity(body.id)) : null;
  try {
    await api('POST', '/api/team/activity', body);
  } catch (err) {
    const c = err.conflict;
    if (!c || !c.prior) {
      throw err;
    }
    if (!confirm(`“${body.prettyId}” is the address of “${c.title}” from ${c.year}. Rename that one to “${c.renamed}” and use “${body.prettyId}” here?`)) {
      throw new Error('Pick another address, or agree to rename the old one.');
    }
    await api('POST', '/api/team/activity', {...body, takeOver: true});
  }
  await reload();
  const now = body.id ? activity(body.id) : null;
  if (before && now && location.pathname === before && activityPath(now) !== before) {
    history.replaceState(null, '', activityPath(now));
    const {render} = await import('./app.js');
    render();
  }
}

function settingCard(label, hint, ...controls) {
  const card = el('div', 'setting-card');
  card.append(el('div', 'setting-label', label), el('div', 'setting-hint', hint));
  for (const c of controls) {
    card.append(c);
  }
  return card;
}

function settingRow(label, hint, control) {
  const row = el('div', 'setting-row');
  const text = el('div', 'setting-text');
  text.append(el('div', 'setting-label', label));
  if (hint) {
    text.append(el('div', 'setting-hint', hint));
  }
  const side = el('div', 'setting-control');
  side.append(control);
  row.append(text, side);
  return row;
}

async function goTo(path) {
  const {navigate} = await import('./app.js');
  navigate(path);
}

let asked = null;

export function people() {
  asked = asked || api('GET', '/api/team/people').catch(err => {
    asked = null;
    throw err;
  });
  return asked;
}

export async function personInfo(email) {
  const all = await people();
  return all.find(p => p.email === email) || null;
}

export async function openPerson(v, node) {
  const info = await personInfo(v.email);
  const head = personHead(v, info);
  const contact = personContact(v, info);
  if (!node || !(node.canEdit || isFamily(v.email)) || !v.position) {
    const done = el('button', 'button button-secondary', 'Done');
    done.type = 'button';
    done.addEventListener('click', closeModal);
    openModal('', [head, contact, personFoot(v, info, done)], {actions: false, wide: 'person'});
    return;
  }
  const form = signUpForm(node, v);
  const tabs = tabbedFields([
    {label: 'Sign up', icon: svg('edit'), fields: form.fields},
    {label: 'Contact', icon: svg('people'), fields: [contact, personFoot(v, info)]},
  ]);
  openModal('', [head, tabs], {...form, wide: 'person', replace: true});
}

function personHead(v, info) {
  const head = el('div', 'who-head');
  const face = el('div', 'avatar who-face');
  const photo = (info && info.photoUrl) || v.photoUrl;
  if (photo) {
    const img = el('img');
    img.src = photo;
    img.alt = '';
    face.append(img);
  } else {
    face.textContent = (v.name || v.email).slice(0, 1).toUpperCase();
  }
  const names = el('div', 'who-names');
  names.append(el('div', 'who-name', (info && info.name) || v.name || v.email));
  if (info && info.pronouns) {
    names.append(el('div', 'who-sub', info.pronouns));
  }
  if (info) {
    const place = info.isStudent
      ? [info.grade, info.classroom].filter(Boolean).join(' · ')
      : [info.jobTitle, info.department].filter(Boolean).join(' · ') || info.title;
    if (place) {
      names.append(el('div', 'who-sub', place));
    }
  }
  const actions = el('div', 'who-actions');
  const action = (icon, label, href, onClick) => {
    const a = el(href ? 'a' : 'button', 'who-action');
    if (href) {
      a.href = href;
    } else {
      a.type = 'button';
      a.addEventListener('click', onClick);
    }
    a.title = label;
    const round = el('span', 'icon-round');
    round.append(svg(icon));
    a.append(round, el('span', 'who-action-label', label));
    actions.append(a);
  };
  action('mail', 'Email', `mailto:${v.email}`);
  const phone = info && info.phone ? info.phone : '';
  const digits = phone.replace(/[^+\d]/g, '');
  if (digits) {
    action('chat', 'Message', `sms:${digits}`);
    action('phone', 'Call', `tel:${digits}`);
  }
  action('copy', 'Copy info', null, () => {
    const lines = [(info && info.name) || v.name || '', v.email, phone].filter(Boolean);
    navigator.clipboard.writeText(lines.join('\n')).then(() => toast('Contact info copied'), () => toast('Could not copy'));
  });
  head.append(face, names, actions);
  return head;
}

function personContact(v, info) {
  const card = el('div', 'who-rows');
  let group = null;
  const row = (icon, label, value) => {
    if (!group) {
      group = el('div', 'who-group');
      card.append(group);
    }
    const r = el('div', 'who-row');
    r.append(svg(icon), el('span', 'who-label', label), typeof value === 'string' ? el('span', 'who-value', value) : value);
    group.append(r);
  };
  const divide = () => {
    group = null;
  };
  row('mail', 'Email', v.email);
  if (info && info.phone) {
    row('phone', 'Phone', info.phone);
  }
  const chips = list => {
    const wrap = el('div', 'who-card-chips');
    for (const p of list) {
      const chip = el('button', 'who-card-chip', p.grade ? `${p.name} (${p.grade})` : p.name);
      chip.type = 'button';
      chip.addEventListener('click', () => openPerson({email: p.email, name: p.name}));
      wrap.append(chip);
    }
    return wrap;
  };
  const household = info && ((info.spouses && info.spouses.length) || (info.children && info.children.length) || (info.isStudent && info.parentEmails && info.parentEmails.length));
  if (household) {
    divide();
  }
  if (info && info.spouses && info.spouses.length) {
    row('people', info.spouses.length === 1 ? 'Partner' : 'Partners', chips(info.spouses));
  }
  if (info && info.children && info.children.length) {
    row('person', info.children.length === 1 ? 'Child' : 'Children', chips(info.children));
  }
  if (info && info.isStudent && info.parentEmails && info.parentEmails.length) {
    const parents = el('div');
    for (const e of info.parentEmails) {
      const a = el('a', 'who-card-link', e);
      a.href = `mailto:${e}`;
      parents.append(a);
    }
    row('people', 'Parents', parents);
  }
  return card;
}

function personFoot(v, info, done) {
  const foot = el('div', 'who-foot');
  if (info) {
    const profile = el('a', 'button', '');
    profile.append(svg('open'), el('span', '', 'Open Helios Who? Profile'));
    profile.href = whoLink(v.email);
    profile.target = '_blank';
    profile.rel = 'noopener';
    foot.append(profile);
  }
  if (done) {
    foot.append(done);
  }
  return foot.children.length ? foot : el('div');
}

export function openSignUp(node, existing, someoneElse) {
  const form = signUpForm(node, existing, someoneElse);
  openModal(existing ? `Edit sign-up for ${node.title}` : `Sign up for ${node.title}`, form.fields, form);
}

function signUpForm(node, existing, someoneElse) {
  const editor = node.canEdit;
  const picker = createPersonPicker(el('div'), {people});
  const emailField = field('Who is it?', picker.mount, 'Search the directory');
  const who = segmented([{label: 'Me', value: ''}, {label: 'Someone else', value: 'other'}],
    someoneElse || (existing && existing.email !== me().email) ? 'other' : '', value => {
      emailField.hidden = value !== 'other';
      if (value === 'other') {
        picker.input.focus();
      }
    });
  emailField.hidden = who.value !== 'other';
  const isChair = Boolean(existing && existing.position === 'Co-Chair');
  const positions = [{label: 'Volunteer', value: 'Volunteer'}];
  if (node.coLeaderNeeded || (existing && existing.position === 'Open to Co-Chair')) {
    positions.push({label: 'Volunteer, and open to co-chairing', value: 'Open to Co-Chair'});
  }
  if (editor) {
    positions.push({label: 'Co-Chair', value: 'Co-Chair'});
  }
  const position = select(positions, existing && (editor || !isChair) ? existing.position : 'Volunteer');
  const note = textarea(existing ? existing.note : '', 3);
  const fields = [];
  if (!existing) {
    fields.push(field('Who', who.wrap), emailField);
  }
  let where = null;
  if (existing) {
    const options = [];
    const offer = (n, group) => {
      if (n === node || n.directSignUp || n.canEdit) {
        options.push({label: n.title, value: n.id, group});
      }
    };
    const up = parentOf(node);
    options.push({label: node.title, value: node.id, group: 'This'});
    if (up) {
      offer(up, 'Part of');
    }
    for (const c of node.children) {
      offer(c, 'Under it');
    }
    for (const s of (up ? up.children : state.model.activities.filter(a => a.year === node.year))) {
      if (s !== node) {
        offer(s, up ? 'Alongside it' : 'Other events');
      }
    }
    if (options.length > 1) {
      where = el('select');
      let groupEl = null;
      let lastGroup = null;
      for (const o of options) {
        if (o.group !== lastGroup) {
          groupEl = el('optgroup');
          groupEl.label = o.group;
          where.append(groupEl);
          lastGroup = o.group;
        }
        const opt = el('option', '', o.label);
        opt.value = o.value;
        opt.selected = o.value === node.id;
        groupEl.append(opt);
      }
      fields.push(field('Signed up for', where, 'Pick another to move this sign-up there.'));
    }
  }
  if (isChair && !editor) {
    fields.push(field('Availability', el('div', 'field-static', 'Co-Chair')));
  } else {
    fields.push(field('Availability', position));
  }
  note.placeholder = 'Anything the organizers and whoever is signed up should know';
  const noteLabel = el('span', '', 'Note ');
  noteLabel.append(el('small', '', '(optional)'));
  fields.push(field(noteLabel, note));
  if (existing && editor) {
    const card = el('div', 'appoint-card');
    const icon = el('div', 'appoint-icon');
    icon.append(svg('people'));
    const text = el('div', 'setting-text');
    text.append(el('div', 'setting-label', isChair ? 'Co-chair' : 'Make co-chair'),
      el('div', 'setting-hint', isChair ? `${existing.name} runs this with you.` : 'Give this person co-chair permissions for this role.'));
    const appoint = button(isChair ? 'Remove as co-chair' : 'Make co-chair', isChair ? 'close' : 'plus',
      'button button-secondary button-small', async () => {
        try {
          await api('POST', '/api/team/volunteer', {
            id: node.id, email: existing.email, position: isChair ? 'Volunteer' : 'Co-Chair', note: note.value,
          });
          closeModal();
          await reload();
          toast(isChair ? `${existing.name} is no longer a co-chair` : `${existing.name} is now a co-chair`);
        } catch (err) {
          toast(err.message);
        }
      });
    card.append(icon, text, appoint);
    fields.push(card);
  }
  return {
    fields,
    saveLabel: existing ? 'Save' : 'Sign Up',
    submit: () => {
      if (!existing && who.value === 'other' && !picker.value) {
        throw new Error('Pick who to sign up from the directory');
      }
      return api('POST', '/api/team/volunteer', {
        id: where ? where.value : node.id,
        from: where && where.value !== node.id ? node.id : '',
        email: existing ? existing.email : (who.value === 'other' ? picker.value : ''),
        position: isChair && !editor ? 'Co-Chair' : position.value, note: note.value,
      });
    },
    onDelete: existing ? () => api('DELETE', '/api/team/volunteer', {id: node.id, email: existing.email}) : null,
    deleteLabel: 'Remove',
    confirmDelete: existing ? `Remove ${existing.name} from ${node.title}?` : '',
  };
}

export async function removeVolunteer(node, volunteer) {
  if (!confirm(`Remove ${volunteer.name} from ${node.title}?`)) {
    return;
  }
  try {
    await api('DELETE', '/api/team/volunteer', {id: node.id, email: volunteer.email});
    await reload();
  } catch (err) {
    toast(err.message);
  }
}

export function highlightInputs(current) {
  const wrap = el('div', 'highlight-inputs');
  const headline = text(current ? current.headline : '', {maxLength: 80, placeholder: 'Performances'});
  const body = textarea(current ? current.body : '', 4);
  body.placeholder = 'A few lines people should not miss. **Double stars** make words bold.';
  const icon = text(current ? current.icon || '' : '', {maxLength: 8, placeholder: '📣'});
  icon.classList.add('highlight-icon-input');
  const picks = el('div', 'emoji-picks');
  const usual = [
    '📣', '📢', '⭐', '✨', '⚠️', '❗', '💡', '✅', '📌', '📅', '⏰', '🗓️',
    '🎉', '🎊', '🎈', '🎂', '🎁', '❤️', '🙏', '👏', '🤝', '🙋', '👋', '👨‍👩‍👧',
    '🎭', '🎶', '🎤', '🎨', '📸', '🎬', '📚', '✏️', '🏫', '🎓', '🔬', '🧩',
    '🍕', '🍪', '☕', '🧁', '🥗', '🍎', '🌮', '🍜', '🍿', '🧃',
    '🏃', '⚽', '🚴', '🥾', '🌳', '🌸', '🌍', '☀️', '🌧️', '🔥',
    '🛠️', '🧹', '📦', '🚗', '🅿️', '🎟️', '💰', '🛍️', '🧺', '🪑',
  ];
  const paintPicks = () => {
    picks.querySelectorAll('.emoji-pick').forEach(b => b.classList.toggle('is-active', b.textContent === icon.value));
  };
  for (const e of usual) {
    const b = el('button', 'emoji-pick', e);
    b.type = 'button';
    b.addEventListener('click', () => {
      icon.value = e;
      paintPicks();
    });
    picks.append(b);
  }
  icon.addEventListener('input', paintPicks);
  paintPicks();
  const iconRow = el('div', 'emoji-row');
  iconRow.append(icon, picks);
  wrap.append(field('Headline', headline), field('Body', body), field('Emoji', iconRow));
  wrap.focus = () => headline.focus();
  return {
    wrap,
    clear: () => {
      headline.value = '';
      body.value = '';
      icon.value = '';
      paintPicks();
    },
    value: () => {
      if (!headline.value.trim() && !body.value.trim()) {
        return null;
      }
      return {headline: headline.value.trim(), body: body.value.trim(), icon: icon.value.trim()};
    },
  };
}

function highlightFields(current) {
  const wrap = el('div', 'highlight-fields');
  const inputs = highlightInputs(current);
  const add = button('Add Highlight Section', 'plus', 'button button-secondary button-small', () => {
    section.hidden = false;
    add.hidden = true;
    inputs.wrap.focus();
  });
  const section = el('div', 'setting-card highlight-editor');
  section.hidden = !current;
  add.hidden = Boolean(current);
  const remove = button('Remove highlight', 'trash', 'button button-secondary button-small', () => {
    inputs.clear();
    section.hidden = true;
    add.hidden = false;
  });
  section.append(el('div', 'setting-label', 'Highlight section'),
    el('div', 'setting-hint', 'A callout under the description for the one thing people must read.'),
    inputs.wrap, remove);
  wrap.append(add, section);
  return {
    wrap,
    value: () => (section.hidden ? null : inputs.value()),
  };
}

function whenFields(act, parent) {
  const own = act ? act.own : {start: '', end: '', timing: ''};
  const when = whenEditor(own.start, own.end, own.timing, parent, 'rows');
  when.wrap.classList.add('field');
  return when;
}

export function openActivity(act, options) {
  const opts = options || {};
  const admin = isAdmin();
  const suggesting = !act && !admin && !(opts.parent && opts.parent.canEdit);
  const yearOptions = allYears().map(y => ({label: y, value: y}));
  const known = new Set(yearOptions.map(y => y.value));
  for (const y of [years().current, years().next]) {
    if (!known.has(y)) {
      yearOptions.unshift({label: y, value: y});
    }
  }
  const year = select(yearOptions, act ? act.year : years().current);
  const title = text(act ? act.title : '', {required: true, maxLength: 120});
  const under = act ? act.parent : (opts.parent ? opts.parent.id : '');
  const root = act ? rootOf(act) : (opts.parent ? rootOf(opts.parent) : null);
  const yearOf = act ? act.year : (opts.parent ? opts.parent.year : year.value);
  const roots = state.model.activities.filter(a => a.year === yearOf);
  const currentParent = under ? activity(under) : null;
  const grandparent = currentParent ? parentOf(currentParent) : null;
  const level = currentParent ? (grandparent ? grandparent.children : roots) : roots;
  const mine = new Set(act ? [act.id, ...descendants(act).map(d => d.id)] : []);
  const parents = admin || !under ? [{label: 'Nothing - this stands on its own', value: ''}] : [];
  for (const n of level) {
    if (!mine.has(n.id) && (admin || n.canEdit || n.id === under)) {
      parents.push({label: n.title, value: n.id});
    }
  }
  const parentSelect = select(parents, under);
  const moveLink = el('button', 'link-button move-under', 'Move under an event…');
  moveLink.type = 'button';
  moveLink.addEventListener('click', () => {
    moveLink.hidden = true;
    underField.hidden = false;
    parentSelect.focus();
  });
  const underField = field('Parent Event', parentSelect, 'What this is part of, if anything');
  const runsHere = currentParent || root;
  const editor = admin || Boolean(runsHere && runsHere.canEdit);
  const headings = headingChoices(c => editor || canAdd(c)).filter(c => editor || c.value !== UNCATEGORIZED);
  const category = select(headings,
    act ? act.category : (opts.category || (headings[0] ? headings[0].value : '')));
  const own = root ? eventCategories(root).filter(c => editor || canAdd(c)) : [];
  const eventCategory = select([{label: 'None', value: ''}, ...own.map(c => ({label: c.title, value: c.id}))],
    act ? act.category : (opts.category || ''));
  const status = select(['Pending', 'Open', 'Done', 'Hidden'], act ? act.status : 'Open');
  const policyDot = (sel, blank) => {
    const wrap = el('div', 'status-select policy-select');
    const paint = () => {
      wrap.dataset.policy = sel.value || blank;
    };
    sel.addEventListener('change', paint);
    paint();
    wrap.append(sel);
    return wrap;
  };
  const dotted = sel => {
    const wrap = el('div', 'status-select');
    wrap.dataset.status = sel.value;
    sel.addEventListener('change', () => {
      wrap.dataset.status = sel.value;
    });
    wrap.append(sel);
    return wrap;
  };
  const description = textarea(act ? act.description : '', 6);
  const when = whenFields(act, act ? parentOf(act) : opts.parent || null);
  const timing = text(act ? act.own.timing : '', {placeholder: 'All Year, Late February, A few times per year'});
  const spots = text(act && act.spots ? String(act.spots) : '', {type: 'number'});
  spots.min = '1';
  spots.max = '99';
  const unlimited = checkbox('Unlimited spots', !(act && act.spots), 'Allow unlimited volunteers to sign up.');
  const spotsRow = el('div', 'field-unit');
  spotsRow.append(spots, el('span', 'field-unit-label', 'people'));
  const spotsField = settingRow('Volunteer spots', 'How many volunteers can sign up for this activity?', el('div', 'setting-stack'));
  spotsField.querySelector('.setting-stack').append(spotsRow, unlimited.wrap);
  unlimited.wrap.classList.add('is-compact');
  const paintSpots = () => {
    spotsRow.hidden = unlimited.input.checked;
    if (!unlimited.input.checked && !spots.value) {
      spots.value = act && act.spots ? String(act.spots) : '10';
    }
  };
  unlimited.input.addEventListener('change', () => {
    paintSpots();
    if (!unlimited.input.checked) {
      spots.focus();
    }
  });
  paintSpots();
  const coLeader = checkbox('Co-chair needed', act ? act.coLeaderNeeded : true,
    'This lets people offer to be a co-chair; you still confirm them as co-chairs.');
  const hidden = checkbox('Keep volunteers secret',
    act ? act.volunteersHidden : Boolean(opts.parent && opts.parent.volunteersHidden),
    'E.g., hide room parent applications, which are secret.');
  const complete = checkbox('Volunteers complete', act ? Boolean(act.volunteersComplete) : false,
    'Say the volunteers are all set, whatever the count; this shows a Volunteers Complete badge and stops sign-ups.');
  const direct = checkbox(under ? 'Allow volunteers for this itself' : 'Allow volunteers for the event itself', act ? act.directSignUp : true,
    under ? 'Unchecking this will allow volunteers for subcommittees, but not this itself.' : 'Unchecking this will allow volunteers for subcommittees, but not the event itself.');
  const signMe = select([
    {label: 'Choose one…', value: ''},
    {label: 'Volunteer', value: 'Volunteer'},
    {label: 'Volunteer, and offer to co-chair', value: 'Open to Co-Chair'},
    {label: 'Nothing', value: 'none'},
  ], '');
  signMe.required = true;
  const signMeRow = act ? null : settingRow('Sign me up as', 'Whether you are on this yourself.', signMe);
  const inheritLabel = under ? 'Same as the parent' : 'No, unless a category says otherwise';
  const allowAdding = select([
    {label: inheritLabel, value: ''},
    {label: 'Yes - people can add, and it goes live', value: ADDING.yes},
    {label: 'Approval needed - people can add, an admin approves', value: ADDING.approval},
    ...(under ? [{label: 'No - only organizers add here', value: ADDING.no}] : []),
  ], act ? act.allowAddingOwn || '' : '');
  const allowAddingRow = settingRow('Allow adding subactivities',
    'Can users add subactivities? Note that this is a default and can be overwritten by the settings of a category.',
    policyDot(allowAdding, under ? 'inherit' : ADDING.no));
  const image = imagePicker(act ? act.image : '', act ? act.imageUrl : '', {query: () => title.value.trim()});
  const suggestImage = imagePicker('', '', {dropzone: true, query: () => title.value.trim()});
  const flyer = imagePicker(act ? act.flyer : '', act ? act.flyerUrl : '', {plain: true});
  const pretty = text(act ? act.prettyId || '' : '', {placeholder: 'applause', maxLength: 40});
  const addressBase = () => `${location.origin}${under ? activityPath(root) + '/' : '/v/'}`;
  const addressLine = el('div', 'address-line');
  const addressText = el('code', 'address-text');
  const addressCopy = button('Copy', 'copy', 'button button-secondary button-small', () => {
    navigator.clipboard.writeText(addressText.textContent).then(() => toast('Address copied'), () => toast('Could not copy'));
  });
  const paintAddress = () => {
    const slug = pretty.value.trim().toLowerCase();
    addressText.textContent = addressBase() + (slug || (act ? act.id : '…'));
    addressCopy.disabled = !act && !slug;
  };
  pretty.addEventListener('input', paintAddress);
  paintAddress();
  addressLine.append(addressText, addressCopy);
  const fields = [field('Title', title)];
  const yearField = (admin || !act) && !under ? settingRow('School year', 'The year this event belongs to.', year) : null;
  if (suggesting) {
    const policy = opts.category ? categoryOf(opts.category) : opts.parent;
    const live = under && ((opts.parent && opts.parent.canEdit) || (policy && policy.allowAdding === ADDING.yes));
    const lead = el('p', 'field-lead suggest-lead', under
      ? `Adding to ${root.title}. ${live ? 'It will go live right away.' : 'The organizers will take a look before it goes live.'}`
      : 'Have an idea for something the HCA could do? Tell us about it and an organizer will take a look.');
    fields.unshift(lead);
  } else {
    if (under) {
      fields.push(underField);
    }
    if (under) {
      if (own.length) {
        fields.push(field('Category', eventCategory, `One of ${root.title}'s own categories`));
      }
    } else {
      fields.push(field('Category', category));
    }
  }
  if (!under && !suggesting && parents.length > 1) {
    underField.hidden = true;
    const slot = el('div', 'field field-link-slot');
    slot.append(moveLink);
    fields.push(slot, underField);
  }
  let statusSelect = null;
  if (admin && act) {
    statusSelect = status;
  } else if (act && act.status === 'Pending') {
    if (currentParent && currentParent.canEdit) {
      statusSelect = select(['Pending', 'Open', 'Done'], 'Pending');
    }
  } else if (act && act.status === 'Hidden') {
    statusSelect = select(['Hidden', 'Open', 'Done'], 'Hidden');
  } else if (act) {
    statusSelect = select(['Open', 'Done'], act.status === 'Done' ? 'Done' : 'Open');
  }
  const statusRow = statusSelect ? settingRow('Status', 'Control whether this activity is open for sign-ups.', dotted(statusSelect)) : null;
  fields.push(field('Description', description));
  const highlight = highlightFields(act ? act.highlight : null);
  let body;
  if (!suggesting) {
    const basics = [settingRow('Title', 'What this is called.', title)];
    if (under) {
      basics.push(settingRow('Parent Event', 'What this is part of.', parentSelect));
      if (own.length) {
        basics.push(settingRow('Category', `One of ${root.title}'s own categories.`, eventCategory));
      }
    } else {
      const where = el('div', 'setting-stack');
      where.append(category);
      if (parents.length > 1) {
        where.append(moveLink, underField);
        underField.classList.add('is-compact');
      }
      basics.push(settingRow('Category', 'Where this is listed on the Opportunities page.', where));
    }
    const about = settingRow('Description', 'What people should know before they sign up.', description);
    about.classList.add('is-stacked');
    basics.push(about, highlight.wrap);
    body = [tabbedFields([
      {label: 'Basics', icon: svg('doc'), fields: basics},
      {label: 'When', icon: svg('calendar'), fields: [...(yearField ? [yearField] : []), when.wrap]},
      {label: 'Sign-ups', icon: svg('people'), fields: [...(statusRow ? [statusRow] : []), spotsField, complete.wrap, allowAddingRow, coLeader.wrap, hidden.wrap, direct.wrap]},
      {label: 'Image & Address', icon: svg('image'), fields: [
        settingCard('Top Banner Image', 'The wide picture across the top of the page and on the card (optional).', image.wrap.querySelector('.image-row')),
        settingCard('Flyer', 'The event\'s poster, shown beside the details and used for the social share image when there is one (optional).', flyer.wrap.querySelector('.image-row')),
        settingCard('Friendly address', under
          ? 'A short address for this activity, under its event. Letters, digits and hyphens; unique among the things beside it.'
          : 'A short address for this event. Letters, digits and hyphens; one address per event, across every year.',
        pretty, addressLine),
      ]},
    ])];
    if (signMeRow) {
      body.push(signMeRow);
    }
  } else {
    description.rows = 4;
    description.placeholder = 'What is it, and what would volunteers do?';
    if (under) {
      if (own.length) {
        fields.splice(fields.indexOf(fields.find(f => f.querySelector && f.querySelector('textarea'))), 0, field('Category', eventCategory));
      }
      fields.push(suggestImage.wrap);
    } else {
      timing.placeholder = 'Spring, a Friday in March, a few times a year…';
      fields.push(field('When (optional)', timing, 'Roughly - the organizers will pin it down with you.'));
    }
    fields.push(signMeRow);
    body = fields;
  }
  openModal(act ? 'Edit Activity' : (suggesting ? (under ? 'Add New Activity' : 'Suggest an Idea') : (opts.parent ? `Add under ${opts.parent.title}` : 'Add Activity')), body, {
    saveLabel: suggesting ? (under ? 'Add' : 'Suggest') : (act ? 'Save changes' : 'Add'),
    deleteLabel: 'Delete activity',
    wide: !suggesting,
    submit: async () => {
      const scheduled = suggesting ? {start: '', end: '', timing: timing.value} : when.value();
      if (!suggesting && when.validate(scheduled)) {
        throw new Error(when.validate(scheduled));
      }
      const body = {
        id: act ? act.id : '',
        year: year.value, title: title.value, parent: parentSelect.value,
        category: parentSelect.value ? eventCategory.value : category.value,
        status: statusSelect ? statusSelect.value : '',
        description: description.value, image: suggesting && under ? suggestImage.value() : image.value(), flyer: flyer.value(), timing: scheduled.timing,
        highlight: highlight.value(),
        start: scheduled.start, end: scheduled.end, location: act ? act.location || '' : '', spots: unlimited.input.checked ? 0 : Number(spots.value) || 0,
        coLeaderNeeded: coLeader.input.checked, volunteersHidden: hidden.input.checked, volunteersComplete: complete.input.checked, directSignUp: direct.input.checked,
        priority: Boolean(act && act.priority),
        signUp: act ? '' : signMe.value, prettyId: pretty.value.trim().toLowerCase(), allowAdding: allowAdding.value,
      };
      await saveActivity(body);
      if (!act) {
        await reload();
        const made = [...allNodes()].find(n => n.year === body.year && n.title === body.title && n.parent === body.parent);
        if (made) {
          await goTo(activityPath(made));
        }
      }
    },
    onDelete: act && admin ? () => api('DELETE', '/api/team/activity', {id: act.id}) : null,
    confirmDelete: act ? `Delete “${act.title}” (${act.year})? Its links go with it.` : '',
    afterDelete: () => goTo(currentParent ? activityPath(currentParent) : '/'),
  });
}

export function openLink(node, item) {
  const title = text(item ? item.title : '', {required: true, maxLength: 120});
  const url = text(item ? item.url : '', {type: 'url', required: true, placeholder: 'https://'});
  const description = textarea(item ? item.description || '' : '', 2);
  const image = imagePicker(item ? item.image : '', item ? item.imageUrl : '');
  openModal(item ? 'Edit Resource' : 'Add Resource', [
    field('Title', title), field('URL', url),
    field('Description', description, 'A line about what people will find there'),
    image.wrap,
  ], {
    submit: () => api('POST', '/api/team/link', {
      id: node.id, original: item ? item.title : '',
      title: title.value, url: url.value, description: description.value, image: image.value(),
    }),
    onDelete: item ? () => api('DELETE', '/api/team/link', {id: node.id, title: item.title}) : null,
    confirmDelete: item ? `Remove the link “${item.title}”?` : '',
  });
}

export function openVolunteerGrid(root, nodes, pathOf) {
  const rows = [];
  for (const node of nodes) {
    for (const v of node.volunteers) {
      rows.push({node, v, parents: []});
    }
  }
  const columns = [
    {label: 'Volunteer', get: r => r.v.name || r.v.email},
    {label: 'Where', get: r => pathOf(r.node)},
    {label: 'As', get: r => r.v.position},
    {label: 'Sign Up Date', get: r => r.v.added || '', show: r => (r.v.added ? longDate(r.v.added) : '')},
    {label: 'Email', get: r => r.v.email},
    {label: "Parents' email", get: r => r.parents.join(', ')},
  ];
  const copied = (btn, icon, label) => {
    btn.classList.add('copied');
    btn.replaceChildren(svg('check'));
    setTimeout(() => {
      btn.classList.remove('copied');
      btn.replaceChildren(svg(icon));
      if (label) {
        btn.append(el('span', '', label));
      }
    }, 1200);
  };
  const copyGlyph = (title, text) => {
    const btn = el('button', 'copy-glyph');
    btn.type = 'button';
    btn.title = title;
    btn.append(svg('copy'));
    btn.addEventListener('click', () => {
      navigator.clipboard.writeText(text()).then(() => copied(btn, 'copy'), () => toast('Could not copy'));
    });
    return btn;
  };
  const table = el('table', 'roster');
  const head = el('tr');
  let sortedBy = null;
  let ascending = true;
  const sortBy = (c, th) => {
    ascending = sortedBy === c ? !ascending : true;
    sortedBy = c;
    rows.sort((a, b) => {
      const x = c.get(a);
      const y = c.get(b);
      if (!x || !y) {
        return x ? -1 : y ? 1 : 0;
      }
      const order = x.localeCompare(y, undefined, {numeric: true, sensitivity: 'base'});
      return ascending ? order : -order;
    });
    for (const cell of head.children) {
      cell.removeAttribute('aria-sort');
    }
    th.setAttribute('aria-sort', ascending ? 'ascending' : 'descending');
    body.append(...rows.map(r => r.tr));
  };
  for (const c of columns) {
    const th = el('th');
    const sort = el('button', 'roster-sort');
    sort.type = 'button';
    sort.title = `Sort by ${c.label}`;
    sort.append(el('span', '', c.label), svg('chevron'));
    sort.addEventListener('click', () => sortBy(c, th));
    th.append(sort, copyGlyph(`Copy the ${c.label} column`, () => rows.map(c.get).filter(Boolean).join('\n')));
    head.append(th);
  }
  const thead = el('thead');
  thead.append(head);
  table.append(thead);
  const body = el('tbody');
  const addresses = new Set();
  const parentCells = [];
  for (const row of rows) {
    const {node, v} = row;
    const tr = el('tr');
    const who = el('td', 'roster-who');
    const face = el('div', 'avatar people-face');
    if (v.photoUrl) {
      const img = el('img');
      img.src = v.photoUrl;
      img.alt = '';
      face.append(img);
    } else {
      face.textContent = (v.name || v.email).slice(0, 1).toUpperCase();
    }
    who.append(face, el('span', '', v.name));
    tr.append(who);
    tr.append(el('td', 'roster-where', pathOf(node)));
    tr.append(el('td', '', v.position));
    tr.append(el('td', 'roster-date', columns.find(c => c.label === 'Sign Up Date').show(row)));
    const mail = el('td', 'roster-mail');
    mail.append(mailto(v.email));
    tr.append(mail);
    addresses.add(v.email);
    const parents = el('td', 'roster-mail');
    parentCells.push({cell: parents, row});
    tr.append(parents);
    row.tr = tr;
    body.append(tr);
  }
  table.append(body);
  people().then(all => {
    const byEmail = new Map(all.map(p => [p.email, p]));
    for (const {cell, row} of parentCells) {
      const info = byEmail.get(row.v.email);
      if (!info || !info.isStudent) {
        continue;
      }
      row.parents = info.parentEmails || [];
      for (const e of row.parents) {
        cell.append(mailto(e));
        addresses.add(e);
      }
      if (!row.parents.length) {
        cell.append(el('span', 'roster-none', 'none listed'));
      }
    }
  });
  const scroll = el('div', 'roster-scroll');
  scroll.append(rows.length ? table : el('div', 'panel-empty', 'Nobody has signed up yet.'));
  const tools = el('div', 'roster-tools');
  tools.append(el('span', 'roster-count', `${rows.length} sign-up${rows.length === 1 ? '' : 's'}`));
  const buttons = el('div', 'roster-buttons');
  const copyTable = button('Copy table', 'copy', 'button button-secondary button-small', () => {
    const text = [columns.map(c => c.label).join('\t'), ...rows.map(r => columns.map(c => c.get(r)).join('\t'))].join('\n');
    navigator.clipboard.writeText(text).then(() => copied(copyTable, 'copy', 'Copy table'), () => toast('Could not copy'));
  });
  copyTable.title = 'Copy every column, ready to paste into a spreadsheet';
  const copyEmails = button('Copy emails', 'copy', 'button button-secondary button-small', () => {
    navigator.clipboard.writeText([...addresses].join(', ')).then(() => copied(copyEmails, 'copy', 'Copy emails'), () => toast('Could not copy'));
  });
  copyEmails.title = "Every volunteer's address, and their parents' for students, comma-separated";
  buttons.append(copyTable, copyEmails);
  tools.append(buttons);
  openModal(`${root.title}: Volunteers`, [tools, scroll], {wide: true});
}

function mailto(email) {
  const a = el('a', 'roster-link', email);
  a.href = `mailto:${email}`;
  return a;
}

export function editPencil(label) {
  const pencil = el('button', 'edit-icon');
  pencil.type = 'button';
  pencil.title = label;
  pencil.setAttribute('aria-label', label);
  pencil.append(svg('edit'));
  return pencil;
}

export function fieldEditor(anchor, pencil, opts) {
  const box = el('div', 'field-editor');
  box.append(opts.input);
  if (opts.hint) {
    box.append(el('small', 'field-note', opts.hint));
  }
  const actions = el('div', 'field-editor-actions');
  const save = el('button', 'button button-small', 'Save');
  save.type = 'button';
  const cancel = el('button', 'button button-secondary button-small', 'Cancel');
  cancel.type = 'button';
  const status = el('span', 'field-status');
  actions.append(save, cancel, status);
  box.append(actions);
  const close = () => {
    box.remove();
    anchor.hidden = false;
    pencil.hidden = false;
  };
  cancel.addEventListener('click', close);
  save.addEventListener('click', async () => {
    const value = opts.value();
    const problem = opts.validate ? opts.validate(value) : '';
    if (problem) {
      status.classList.add('error');
      status.textContent = problem;
      return;
    }
    status.classList.remove('error');
    save.disabled = true;
    status.textContent = 'Saving…';
    await opts.submit(value);
    save.disabled = false;
    status.textContent = '';
  });
  box.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
      e.stopPropagation();
      close();
    }
    if (e.key === 'Enter' && e.target.tagName !== 'TEXTAREA') {
      e.preventDefault();
      save.click();
    }
  });
  anchor.hidden = true;
  pencil.hidden = true;
  anchor.after(box);
  opts.input.focus();
}

export function editable(anchor, label, make, submit) {
  const pencil = editPencil(label);
  pencil.addEventListener('click', () => {
    const built = make();
    fieldEditor(anchor, pencil, {
      input: built.input,
      hint: built.hint,
      value: built.value,
      validate: built.validate,
      submit,
    });
  });
  return pencil;
}

export function openCategory(category, eventId, after) {
  const title = text(category ? category.title : '', {required: true, maxLength: 120});
  const description = textarea(category ? category.description : '', 3);
  description.placeholder = 'Add a brief description (optional).';
  const image = imagePicker(category ? category.image : '', category ? category.imageUrl : '',
    {dropzone: true, hint: 'Add an image to represent this category (optional).'});
  const policies = [
    ...(eventId ? [{label: 'Same as the event', value: ''}] : []),
    {label: 'Yes - people can add, and it goes live', value: ADDING.yes},
    {label: 'Approval needed - people can add, an admin approves', value: ADDING.approval},
    {label: 'No - only organizers add here', value: ADDING.no},
  ];
  const adding = select(policies, category ? category.allowAddingOwn || '' : (eventId ? '' : ADDING.no));
  const addingWrap = el('div', 'status-select policy-select');
  const paintAdding = () => {
    addingWrap.dataset.policy = adding.value || 'inherit';
  };
  adding.addEventListener('change', paintAdding);
  paintAdding();
  addingWrap.append(adding);
  const titleField = field('Title', title, 'A short, clear name for this category.');
  titleField.classList.add('is-required');
  const addingField = field('Allow adding', addingWrap, 'Control whether people can add activities to this category.');
  addingField.classList.add('is-required');
  const fields = [titleField, field('Description', description), image.wrap, addingField];
  const onMain = eventId ? null : checkbox('Show on the main page (it stays in the toolbar either way)', category ? category.showOnMain : true);
  if (onMain) {
    fields.push(onMain.wrap);
  }
  openModal(category ? 'Edit Category' : 'Add Category', fields, {
    replace: true,
    saveLabel: category ? 'Save changes' : 'Add',
    submit: () => api('POST', '/api/team/category', {
      id: category ? category.id : '', eventId: eventId || '',
      title: title.value, description: description.value, image: image.value(), allowAdding: adding.value,
      showOnMain: onMain ? onMain.input.checked : true,
    }),
    afterSave: after,
    onDelete: category ? () => api('DELETE', '/api/team/category', {id: category.id}) : null,
    confirmDelete: category ? `Delete the category “${category.title}”?` : '',
    afterDelete: after,
  });
}

function addingWords(category) {
  const own = category.allowAddingOwn || '';
  const word = {[ADDING.yes]: 'People can add here', [ADDING.approval]: 'People can suggest here', [ADDING.no]: 'Only organizers add here'}[category.allowAdding] || '';
  return own ? word : (category.eventId ? `${word} (same as the event)` : word);
}

export function categoryList(root, after) {
  const wrap = el('div', 'category-list');
  const eventId = root ? root.id : '';
  const list = root ? eventCategories(root) : state.model.categories.filter(c => !c.builtIn);
  const move = async (from, to) => {
    const ids = list.map(c => c.id);
    const [moved] = ids.splice(from, 1);
    ids.splice(to, 0, moved);
    try {
      await api('POST', '/api/team/categories/order', {eventId, ids});
      await reload();
      if (after) {
        after();
      }
    } catch (err) {
      toast(err.message);
    }
  };
  list.forEach((category, i) => {
    const row = el('div', 'admin-row');
    if (category.imageUrl) {
      row.append(thumb(category.imageUrl, category.title, 'small'));
    }
    const body = el('div', 'grow');
    body.append(el('div', '', category.title));
    body.append(el('div', 'sub', [category.description, addingWords(category),
      !root && !category.showOnMain ? 'Toolbar only' : ''].filter(Boolean).join(' · ')));
    const up = button('', 'up', 'icon-button', () => move(i, i - 1));
    up.setAttribute('aria-label', `Move ${category.title} up`);
    up.disabled = i === 0;
    const down = button('', 'down', 'icon-button', () => move(i, i + 1));
    down.setAttribute('aria-label', `Move ${category.title} down`);
    down.disabled = i === list.length - 1;
    row.append(body, up, down, button('Edit', 'edit', 'button button-secondary button-small', () => openCategory(category, eventId, after)));
    wrap.append(row);
  });
  if (!list.length) {
    wrap.append(el('div', 'panel-empty', root ? `${root.title} has no categories of its own yet.` : 'No categories yet.'));
  }
  if (!root) {
    wrap.append(el('div', 'hint', 'Uncategorized is built in: it collects events whose category is blank or names nothing here.'));
  }
  const add = el('div', 'add-row');
  add.append(button('Add Category', 'plus', 'button', () => openCategory(null, eventId, after)));
  wrap.append(add);
  return wrap;
}

export function openCategoryManager(root) {
  const again = () => openCategoryManager(root);
  openModal(`${root.title}: Categories`, [
    el('div', 'hint', 'The things under this event are grouped by these, in this order. Each one says whether people may add to it.'),
    categoryList(root, again),
  ], {replace: true});
}

export function openSettings() {
  const settings = state.model.settings;
  const expense = text(settings.expenseFormUrl, {type: 'url', required: true});
  const intro = textarea(settings.intro, 4);
  openModal('Settings', [field('Expense form URL', expense), field('Intro', intro, 'Shown under the Sign Up heading')], {
    submit: () => api('POST', '/api/team/settings', {expenseFormUrl: expense.value, intro: intro.value}),
  });
}

export function openRedirect(item) {
  const old = text(item ? item.old : '', {required: true, placeholder: 'https://hca.heliosian.com/dl/signup/... or /old/path'});
  const to = text(item ? item.new : '', {required: true, placeholder: '/v/spring-celebration, or https://...'});
  openModal(item ? 'Edit Redirect' : 'Add Redirect', [
    field('Old address', old, 'The link people still hold: paste the whole link, or its path. A bare word is a friendly address, /v/word.'),
    field('Send them to', to, 'A page here, as its path, or a whole address on another site. A link into what sits under the old address follows along.'),
  ], {
    submit: () => api('POST', '/api/team/redirect', {original: item ? item.old : '', old: old.value, new: to.value}),
    onDelete: item ? () => api('DELETE', '/api/team/redirect', {old: item.old}) : null,
    confirmDelete: item ? `Remove the redirect from ${item.old}? Anyone holding that link will get Not found.` : '',
    deleteLabel: 'Remove',
  });
}

export async function copyToNextYear(act) {
  if (!confirm(`Copy “${act.title}” and everything under it into ${years().next}?`)) {
    return;
  }
  try {
    await api('POST', '/api/team/copy', {id: act.id});
    await reload();
    toast(`Copied to ${years().next}`);
  } catch (err) {
    toast(err.message);
  }
}

export async function saveActivityFields(act, changes) {
  const body = {
    id: act.id,
    year: act.year, title: act.title, parent: act.parent || '',
    category: act.category || '', status: act.status,
    description: act.description || '', image: act.image || '', flyer: act.flyer || '', timing: act.own.timing,
    start: act.own.start, end: act.own.end, location: act.location || '', spots: act.spots || 0,
    coLeaderNeeded: act.coLeaderNeeded, volunteersHidden: act.volunteersHidden, volunteersComplete: Boolean(act.volunteersComplete), directSignUp: act.directSignUp,
    priority: Boolean(act.priority),
    prettyId: act.prettyId || '', allowAdding: act.allowAddingOwn || '', highlight: act.highlight || null,
    ...changes,
  };
  try {
    await saveActivity(body);
  } catch (err) {
    toast(err.message);
    await reload();
  }
}

