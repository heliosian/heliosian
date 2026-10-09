import {state, me, allows, longDate, whenLabel, years, allYears, activity, parentOf, ADDING, category as categoryOf, descendants, rootOf, eventCategories, headingChoices, UNCATEGORIZED, isFamily} from './state.js';

import {whenEditor, treeFilter} from './dom.js';
import {dataGrid} from '/datagrid.js';
import {familyDropdown} from '/rules.js';
import {el, svg, toast, button, imageThumb} from '/elements.js';
import {imageTools} from '/images.js';
import {createPersonPicker} from '/picker.js';
import {listed, known, emailOf, gradeOf, isStudent, isParent, isStaff, relativesOf} from '/directory.js';
import {openPersonCard} from '/personcard.js';
import {api} from '/api.js';
import {act, create, remove} from '/data.js';
import {openModal, closeModal} from '/modal.js';
import {load, navigate, render} from '/router.js';
import {field, text, textarea, select, checkbox, segmented} from '/form.js';
import {tabbedFields} from '/tabs.js';

export const {uploadImage, uploadAndSave, imageSearchOn, openImageSearch, imagePicker} = imageTools('/api/team', {state});

async function writeActivity(body) {
  const {id, ...fields} = body;
  if (id) {
    await act('activities', id, 'edit', fields);
    return {id};
  }
  if (fields.category && fields.category !== UNCATEGORIZED) {
    return act('activity-categories', fields.category, 'add', fields);
  }
  if (fields.parent) {
    return act('activities', fields.parent, 'add', fields);
  }
  return create('activities', fields);
}

export async function saveActivity(body) {
  const before = body.id && activity(body.id) ? activity(body.id).path : null;
  let saved;
  try {
    saved = await writeActivity(body);
  } catch (err) {
    const c = err.conflict;
    if (!c || !c.prior) {
      throw err;
    }
    if (!confirm(`“${body.prettyId}” is the address of “${c.title}” from ${c.year}. Rename that one to “${c.renamed}” and use “${body.prettyId}” here?`)) {
      throw new Error('Pick another address, or agree to rename the old one.');
    }
    saved = await writeActivity({...body, takeOver: true});
  }
  await load();
  const now = body.id ? activity(body.id) : null;
  if (before && now && location.pathname === before && now.path !== before) {
    history.replaceState(null, '', now.path);
    render();
  }
  return saved.id;
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

export function openPerson(v, node) {
  if (!node || !(node.canEdit || isFamily(v.email)) || !v.position) {
    return openPersonCard(v);
  }
  const form = signUpForm(node, v);
  return openPersonCard(v, [{label: 'Sign up', icon: svg('edit'), fields: form.fields}], {...form, replace: true});
}

export function openSignUp(node, existing, someoneElse) {
  const form = signUpForm(node, existing, someoneElse);
  openModal(existing ? `Edit sign-up for ${node.title}` : `Sign up for ${node.title}`, form.fields, form);
}

function signUpForm(node, existing, someoneElse) {
  const editor = node.canEdit;
  const picker = createPersonPicker(el('div'), {people: listed});
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
          await act('volunteers', existing.id, 'edit', {position: isChair ? 'Volunteer' : 'Co-Chair', note: note.value});
          closeModal();
          await load();
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
      return act('activities', where ? where.value : node.id, 'sign-up', {
        from: where && where.value !== node.id ? node.id : '',
        email: existing ? existing.email : (who.value === 'other' ? picker.value : ''),
        position: isChair && !editor ? 'Co-Chair' : position.value, note: note.value,
      });
    },
    onDelete: existing && existing.can.delete ? () => remove('volunteers', existing.id) : null,
    deleteLabel: 'Remove',
    confirmDelete: existing ? `Remove ${existing.name} from ${node.title}?` : '',
  };
}

export async function removeVolunteer(node, volunteer) {
  if (!confirm(`Remove ${volunteer.name} from ${node.title}?`)) {
    return;
  }
  try {
    await remove('volunteers', volunteer.id);
    await load();
  } catch (err) {
    toast(err.message);
  }
}

function highlightInputs(current) {
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
  const admin = allows('team.curate');
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
  const headings = headingChoices(c => editor || c.can.add).filter(c => editor || c.value !== UNCATEGORIZED);
  const category = select(headings,
    act ? act.category : (opts.category || (headings[0] ? headings[0].value : '')));
  const own = root ? eventCategories(root).filter(c => editor || c.can.add) : [];
  const eventCategory = select([{label: 'None', value: ''}, ...own.map(c => ({label: c.title, value: c.id}))],
    act ? act.category : (opts.category || ''));
  const description = textarea(act ? act.description : '', 6);
  const when = whenFields(act, act ? parentOf(act) : opts.parent || null);
  const timing = text(act ? act.own.timing : '', {placeholder: 'All Year, Late February, A few times per year'});
  const signMe = select([
    {label: 'Choose one…', value: ''},
    {label: 'Volunteer', value: 'Volunteer'},
    {label: 'Volunteer, and offer to co-chair', value: 'Open to Co-Chair'},
    {label: 'Nothing', value: 'none'},
  ], '');
  signMe.required = true;
  const signMeRow = act ? null : settingRow('Sign me up as', 'Whether you are on this yourself.', signMe);
  const suggestImage = imagePicker('', '', {dropzone: true, query: () => title.value.trim()});
  const pretty = text(act ? act.prettyId || '' : '', {placeholder: 'applause', maxLength: 40});
  const addressBase = () => `${location.origin}${under ? root.path + '/' : '/v/'}`;
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
  const describeStatus = el('small', 'describe-status');
  const generate = button('Generate with AI', 'sparkle', 'button button-small button-secondary', async () => {
    generate.disabled = true;
    describeStatus.classList.remove('error');
    describeStatus.textContent = 'Writing…';
    try {
      const parent = parentSelect.value ? activity(parentSelect.value) : null;
      const heading = parentSelect.value ? eventCategory : (suggesting ? null : category);
      const {description: written} = await api('POST', '/api/team/describe', {
        title: title.value,
        parent: parent ? parent.title : '',
        category: heading && heading.value && heading.selectedOptions[0] ? heading.selectedOptions[0].textContent : '',
        when: suggesting ? timing.value : whenLabel(when.value()),
        notes: description.value,
      });
      description.value = written;
      describeStatus.textContent = '';
      description.focus();
    } catch (err) {
      describeStatus.classList.add('error');
      describeStatus.textContent = err.message;
    } finally {
      generate.disabled = false;
    }
  });
  const describeLine = el('div', 'describe-line');
  describeLine.append(generate, describeStatus);
  const describeBox = el('div');
  describeBox.append(description, describeLine);
  fields.push(field('Description', describeBox));
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
    const about = settingRow('Description', 'What people should know before they sign up.', describeBox);
    about.classList.add('is-stacked');
    basics.push(about, highlight.wrap);
    body = [tabbedFields([
      {label: 'Basics', icon: svg('doc'), fields: basics},
      {label: 'When', icon: svg('calendar'), fields: [...(yearField ? [yearField] : []), when.wrap]},
      {label: 'Address', icon: svg('link'), fields: [
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
  const scheduledNow = () => (suggesting ? {start: '', end: '', timing: timing.value} : when.value());
  const formBody = scheduled => ({
    id: act ? act.id : '',
    year: year.value, title: title.value, parent: parentSelect.value,
    category: parentSelect.value ? eventCategory.value : category.value,
    status: act ? act.status : '',
    description: description.value, image: suggesting && under ? suggestImage.value() : (act ? act.image || '' : ''), flyer: act ? act.flyer || '' : '', timing: scheduled.timing,
    highlight: highlight.value(),
    start: scheduled.start, end: scheduled.end, location: act ? act.location || '' : '', spots: act ? act.spots || 0 : 0,
    coLeaderNeeded: act ? Boolean(act.coLeaderNeeded) : true, volunteersComplete: Boolean(act && act.volunteersComplete),
    priority: Boolean(act && act.priority),
    signUp: act ? '' : signMe.value, prettyId: pretty.value.trim().toLowerCase(), allowAdding: act ? act.allowAddingOwn || '' : '',
  });
  const opened = act ? formBody(scheduledNow()) : null;
  openModal(act ? 'Edit Activity' : (suggesting ? (under ? 'Add New Activity' : 'Suggest an Idea') : (opts.parent ? `Add under ${opts.parent.title}` : 'Add Activity')), body, {
    saveLabel: suggesting ? (under ? 'Add' : 'Suggest') : (act ? 'Save changes' : 'Add'),
    deleteLabel: 'Delete activity',
    wide: !suggesting,
    submit: async () => {
      const scheduled = scheduledNow();
      if (!suggesting && when.validate(scheduled)) {
        throw new Error(when.validate(scheduled));
      }
      const body = formBody(scheduled);
      if (act) {
        const changed = Object.keys(body).filter(key => JSON.stringify(body[key]) !== JSON.stringify(opened[key]));
        if (!changed.length) {
          return;
        }
        await saveActivity(Object.fromEntries([['id', act.id], ...changed.map(key => [key, body[key]])]));
        return;
      }
      const saved = await saveActivity(body);
      const made = act ? null : activity(saved);
      if (made) {
        navigate(made.path);
      }
    },
    onDelete: act && act.can.delete ? () => remove('activities', act.id) : null,
    confirmDelete: act ? `Delete “${act.title}” (${act.year})? Its links go with it.` : '',
    afterDelete: () => navigate(currentParent ? currentParent.path : '/'),
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
    submit: () => {
      const fields = {title: title.value, url: url.value, description: description.value, image: image.value()};
      return item ? act('activity-links', item.id, 'edit', fields) : create('activity-links', {activity: node.id, ...fields});
    },
    onDelete: item && item.can.delete ? () => remove('activity-links', item.id) : null,
    confirmDelete: item ? `Remove the link “${item.title}”?` : '',
  });
}

const foldedSettings = new Set();

export function openVolunteerSettings(node, replace) {
  const again = () => openVolunteerSettings(activity(node.id), true);
  const root = rootOf(node);
  const cats = eventCategories(root);
  const list = el('div', 'vol-settings');
  const head = el('div', 'vol-settings-row vol-settings-head');
  const helped = (label, help) => {
    const cell = el('span', 'vol-settings-help');
    const mark = el('span', 'vol-settings-mark');
    mark.title = help;
    mark.append(svg('help'));
    cell.append(el('span', '', label), mark);
    return cell;
  };
  head.append(
    el('span', '', ''),
    el('span', 'vol-settings-head-name', 'Activity'),
    helped('Allow volunteers', 'Allow volunteers for this activity'),
    helped('Show volunteers', 'Show volunteers to everyone. Hidden shows it only to chairs.'),
    helped('Allow new activities', 'Allow new groups/subactivities/roles under this item'),
  );
  list.append(head);
  const folded = key => foldedSettings.has(key);
  const tally = things => {
    const n = things.reduce((sum, c) => sum + 1 + descendants(c).length, 0);
    return el('span', 'vol-settings-count', `${n} ${n === 1 ? 'item' : 'items'}`);
  };
  const fold = (key, has) => {
    if (!has) {
      return el('span', 'vol-fold');
    }
    const b = button('', folded(key) ? 'chevron-right' : 'chevron-down', 'icon-button vol-fold', () => {
      if (folded(key)) {
        foldedSettings.delete(key);
      } else {
        foldedSettings.add(key);
      }
      again();
    });
    b.setAttribute('aria-label', folded(key) ? 'Show what is under it' : 'Hide what is under it');
    return b;
  };
  const yesNo = on => (on ? 'Yes' : 'No');
  const flip = v => ({Yes: 'No', No: 'Yes'})[v] || '';
  const visibility = () => [{label: 'Default', value: ''}, {label: 'Visible', value: 'Yes'}, {label: 'Hidden', value: 'No'}];
  const addingChoices = () => [{label: 'Default', value: ''}, {label: 'Allowed', value: ADDING.yes}, {label: 'Approval needed', value: ADDING.approval}, {label: 'Prohibited', value: ADDING.no}];
  const choice = (options, value, disabledWhy, save, resolved) => {
    const input = select(options, value);
    input.disabled = Boolean(disabledWhy);
    input.title = disabledWhy || '';
    const tint = () => {
      input.dataset.value = ({Yes: 'yes', unlimited: 'yes', limited: 'approval', No: 'no', [ADDING.approval]: 'approval'})[input.value || resolved];
    };
    tint();
    let was = value;
    input.addEventListener('change', async () => {
      tint();
      try {
        await save(input.value);
        was = input.value;
        again();
      } catch (err) {
        toast(err.message);
        input.value = was;
        tint();
      }
    });
    const cell = el('span', 'vol-settings-cell');
    cell.append(input);
    return cell;
  };
  const limitChip = (n, locked, save) => {
    const chip = el('button', 'vol-limit', `${n.spots} ${n.spots === 1 ? 'spot' : 'spots'}`);
    chip.type = 'button';
    chip.title = locked || 'Click to change the limit';
    chip.disabled = Boolean(locked);
    chip.addEventListener('click', () => {
      const input = text(String(n.spots), {type: 'number', min: 1, max: 99, step: 1});
      input.className = 'vol-limit-input';
      let done = false;
      const finish = async keep => {
        if (done) {
          return;
        }
        done = true;
        const spots = Math.max(1, Math.min(99, Math.round(Number(input.value)) || 0));
        if (!keep || spots === n.spots) {
          input.replaceWith(chip);
          return;
        }
        try {
          await save({spots});
          again();
        } catch (err) {
          toast(err.message);
          input.replaceWith(chip);
        }
      };
      input.addEventListener('keydown', e => {
        if (e.key === 'Enter') {
          e.preventDefault();
          finish(true);
        }
        if (e.key === 'Escape') {
          e.preventDefault();
          e.stopPropagation();
          finish(false);
        }
      });
      input.addEventListener('blur', () => finish(true));
      chip.replaceWith(input);
      input.focus();
      input.select();
    });
    return chip;
  };
  const volunteering = (n, own, top, locked, save, resolved) => {
    const options = own([{label: 'Default', value: ''}, {label: 'Unlimited', value: 'unlimited'}, {label: 'Limited', value: 'limited'}, {label: 'No More', value: 'No'}]);
    const allowed = n.directSignUpOwn || (top ? yesNo(n.directSignUp) : '');
    const value = allowed === 'Yes' ? (n.spots > 0 ? 'limited' : 'unlimited') : allowed;
    const changes = {'': {directSignUp: ''}, unlimited: {directSignUp: 'Yes', spots: 0}, limited: {directSignUp: 'Yes', spots: n.spots || 5}, No: {directSignUp: 'No'}};
    const cell = choice(options, value, locked, v => save(changes[v]), resolved);
    cell.classList.add('vol-settings-volunteering');
    if (value === 'limited') {
      cell.append(limitChip(n, locked, save));
    }
    return cell;
  };
  const inherited = n => {
    if (!n.parent) {
      return null;
    }
    return (n.parent === root.id && cats.find(c => c.id === n.category)) || activity(n.parent);
  };
  const saveCategory = async (cat, flags) => {
    await act('activity-categories', cat.id, 'settings', {flags});
    await load();
  };
  const closedAbove = n => {
    for (let a = n; a.parent; a = activity(a.parent)) {
      const cat = a.parent === root.id && cats.find(c => c.id === a.category);
      if (cat && cat.hidden) {
        return cat.title;
      }
      const up = activity(a.parent);
      if (up.status === 'Hidden') {
        return up.title;
      }
    }
    return '';
  };
  const eye = (open, closedBy, disabledWhy, save) => {
    const shown = open && !closedBy;
    const b = button('', shown ? 'eye' : 'eye-off', 'icon-button vol-eye' + (shown ? ' is-open' : '') + (closedBy ? ' is-inherited' : ''), async () => {
      b.disabled = true;
      try {
        await save(!open);
        again();
      } catch (err) {
        toast(err.message);
        b.disabled = false;
      }
    });
    const why = closedBy ? `because ${closedBy} is hidden` : disabledWhy;
    b.disabled = Boolean(why);
    b.title = (shown ? 'Visible' : 'Hidden') + (why ? ` - ${why}` : '');
    b.setAttribute('aria-label', b.title);
    const cell = el('span', 'vol-settings-cell');
    cell.append(b);
    return cell;
  };
  const adder = (placeholder, create) => {
    const box = el('div', 'vol-settings-add');
    const input = text('', {maxLength: 120, placeholder});
    const close = () => box.remove();
    const submit = async () => {
      const title = input.value.trim();
      if (!title) {
        input.focus();
        return;
      }
      input.disabled = true;
      try {
        await create(title);
        again();
      } catch (err) {
        toast(err.message);
        input.disabled = false;
      }
    };
    input.addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        e.preventDefault();
        submit();
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        close();
      }
    });
    const ok = button('', 'check', 'button button-small vol-settings-ok', submit);
    ok.title = 'Add';
    ok.setAttribute('aria-label', 'Add');
    const cancel = button('', 'close', 'button button-secondary button-small vol-settings-ok', close);
    cancel.title = 'Cancel';
    cancel.setAttribute('aria-label', 'Cancel');
    box.append(input, ok, cancel);
    return box;
  };
  const menu = (anchor, items) => {
    const box = el('div', 'vol-add-menu');
    for (const [icon, words, onClick] of items) {
      const b = el('button', 'vol-add-menu-item');
      b.type = 'button';
      b.append(svg(icon), el('span', '', words));
      b.addEventListener('click', () => {
        box.remove();
        onClick();
      });
      box.append(b);
    }
    const at = anchor.getBoundingClientRect();
    box.style.top = `${at.bottom + 4}px`;
    box.style.left = `${at.left}px`;
    document.body.append(box);
    setTimeout(() => document.addEventListener('click', () => box.remove(), {once: true}));
  };
  const addCategoryAfter = after => {
    if (after.nextElementSibling && after.nextElementSibling.classList.contains('vol-settings-add')) {
      after.nextElementSibling.querySelector('input').focus();
      return;
    }
    const box = adder('New category name', title => create('activity-categories', {eventId: node.id, title, allowAdding: ''}).then(load));
    box.classList.add('is-inline');
    box.style.setProperty('--depth', 1);
    after.after(box);
    box.querySelector('input').focus();
  };
  const rename = (title, current, save) => {
    const row = title.closest('.vol-settings-row');
    const dragging = row.draggable;
    row.draggable = false;
    const box = el('span', 'vol-settings-rename');
    const input = text(current, {maxLength: 120});
    const close = () => {
      box.remove();
      title.hidden = false;
      row.draggable = dragging;
    };
    const submit = async () => {
      const next = input.value.trim();
      if (!next || next === current) {
        close();
        return;
      }
      input.disabled = true;
      try {
        await save(next);
        again();
      } catch (err) {
        toast(err.message);
        input.disabled = false;
      }
    };
    input.addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        e.preventDefault();
        submit();
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        close();
      }
    });
    const ok = button('', 'check', 'button button-small vol-settings-ok', submit);
    ok.title = 'Save';
    ok.setAttribute('aria-label', 'Save');
    const cancel = button('', 'close', 'button button-secondary button-small vol-settings-ok', close);
    cancel.title = 'Cancel';
    cancel.setAttribute('aria-label', 'Cancel');
    box.append(input, ok, cancel);
    title.hidden = true;
    title.after(box);
    input.focus();
    input.select();
  };
  const addUnder = (after, parent, category, depth) => {
    if (after.nextElementSibling && after.nextElementSibling.classList.contains('vol-settings-add')) {
      after.nextElementSibling.querySelector('input').focus();
      return;
    }
    const box = adder('New activity name', title => saveActivity({year: parent.year, title, parent: parent.id, category, coLeaderNeeded: true}));
    box.classList.add('is-inline');
    box.style.setProperty('--depth', depth);
    after.after(box);
    box.querySelector('input').focus();
  };
  let dragging = null;
  const zoneClasses = ['is-drop-onto', 'is-drop-before', 'is-drop-after'];
  const dropTarget = (r, item, accepts, drop, between) => {
    const zoneOf = e => {
      if (!between) {
        return 'onto';
      }
      const box = r.getBoundingClientRect();
      const y = (e.clientY - box.top) / box.height;
      if (y < 0.25) {
        return 'before';
      }
      return y > 0.75 ? 'after' : 'onto';
    };
    const clear = () => r.classList.remove(...zoneClasses);
    r.addEventListener('dragover', e => {
      if (dragging && dragging !== item && accepts(dragging)) {
        e.preventDefault();
        clear();
        r.classList.add('is-drop-' + zoneOf(e));
      }
    });
    r.addEventListener('dragleave', clear);
    r.addEventListener('drop', async e => {
      clear();
      if (!dragging || dragging === item || !accepts(dragging)) {
        return;
      }
      e.preventDefault();
      const moved = dragging;
      dragging = null;
      try {
        await drop(moved, zoneOf(e));
        again();
      } catch (err) {
        toast(err.message);
      }
    });
  };
  const draggable = (r, name, item) => {
    r.draggable = true;
    r.classList.add('is-draggable');
    const grip = el('span', 'vol-grip');
    grip.append(svg('grip'));
    name.prepend(grip);
    r.addEventListener('dragstart', e => {
      dragging = item;
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', item.id);
      r.classList.add('is-dragging');
    });
    r.addEventListener('dragend', () => {
      dragging = null;
      r.classList.remove('is-dragging');
    });
  };
  const within = (n, ancestor) => {
    for (let a = n; a; a = a.parent ? activity(a.parent) : null) {
      if (a.id === ancestor.id) {
        return true;
      }
    }
    return false;
  };
  const place = async (moved, parent, category) => {
    const changes = {};
    if (moved.node.parent !== parent.id) {
      changes.parent = parent.id;
    }
    if ((moved.node.category || '') !== category) {
      changes.category = category;
    }
    if (Object.keys(changes).length) {
      await saveActivity({id: moved.node.id, ...changes});
    }
  };
  const nest = (moved, target) => place(moved, target, '');
  const slot = async (moved, target, after) => {
    const parent = activity(target.parent);
    await place(moved, parent, parent.id === root.id ? target.category || '' : '');
    const ids = activity(parent.id).children.map(c => c.id).filter(id => id !== moved.node.id);
    ids.splice(ids.indexOf(target.id) + (after ? 1 : 0), 0, moved.node.id);
    await act('activities', parent.id, 'order', {ids});
    await load();
  };
  const row = (n, depth, below, parent) => {
    const r = el('div', 'vol-settings-row' + (depth ? ' is-child' : ' is-block'));
    const name = el('span', 'vol-settings-name');
    name.style.setProperty('--depth', Math.max(depth - 1, 0));
    const key = 'a:' + n.id;
    const title = el('span', 'vol-settings-title', n.title);
    name.append(fold(key, below ? n.children.length > 0 : n.children.length > 0 || cats.length > 0), title);
    if (folded(key) && n.children.length) {
      name.append(tally(n.children));
    }
    if (n.canEdit) {
      const pencil = editPencil(`Rename ${n.title}`);
      pencil.addEventListener('click', () => rename(title, n.title, next => saveActivity({id: n.id, title: next})));
      const categories = !n.parent && n.id === node.id;
      const plus = button('', 'plus', 'edit-icon', () => {
        if (!categories) {
          addUnder(r, n, '', depth + 1);
          return;
        }
        menu(plus, [
          ['plus', 'Add activity', () => addUnder(r, n, '', depth + 1)],
          ['list', 'Add category', () => addCategoryAfter(r)],
        ]);
      });
      plus.title = categories ? `Add an activity or a category to ${n.title}` : `Add an activity under ${n.title}`;
      plus.setAttribute('aria-label', plus.title);
      name.append(pencil, plus);
    }
    const item = {id: n.id, kind: 'activity', node: n, parent};
    if (parent && parent.canEdit) {
      draggable(r, name, item);
    }
    if (n.canEdit) {
      dropTarget(r, item, d => d.kind === 'activity' && !within(n, d.node),
        (moved, zone) => (zone === 'onto' ? nest(moved, n) : slot(moved, n, zone === 'after')), Boolean(n.parent));
    }
    const locked = n.canEdit ? '' : 'You can’t edit this one';
    let hiding = locked;
    if (!hiding && n.status === 'Pending') {
      hiding = 'Waiting for approval';
    }
    const above = n.parent ? activity(n.parent) : null;
    const source = inherited(n);
    const top = !n.parent;
    const own = options => (top ? options.filter(o => o.value) : options);
    const save = changes => saveActivity({id: n.id, ...changes});
    r.classList.toggle('is-hidden', n.status === 'Hidden' || Boolean(closedAbove(n)));
    r.append(
      eye(n.status !== 'Hidden', closedAbove(n), hiding, open => save({status: open ? 'Open' : 'Hidden'})),
      name,
      volunteering(n, own, top, locked, save, yesNo(source ? source.directSignUp : true)),
      choice(own(visibility()), flip(n.volunteersHiddenOwn) || (top ? yesNo(!n.volunteersHidden) : ''), locked, v => save({volunteersHidden: flip(v)}),
        yesNo(source ? !source.volunteersHidden : true)),
      choice(own(addingChoices()), n.allowAddingOwn || (top ? n.allowAdding || ADDING.no : ''), locked, v => save({allowAdding: v}),
        above ? above.allowAdding || ADDING.no : ADDING.no),
    );
    list.append(r);
    if (!below || folded(key)) {
      return;
    }
    for (const c of n.children) {
      row(c, depth + 1, true, n);
    }
  };
  row(node, 0, false, null);
  const managing = !node.parent;
  const open = !folded('a:' + node.id);
  cats.forEach((cat, i) => {
    const group = node.children.filter(c => (c.category || '') === cat.id);
    if (!open || (!group.length && !managing)) {
      return;
    }
    const heading = el('div', 'vol-settings-row vol-settings-group is-block is-nested');
    const name = el('span', 'vol-settings-name');
    name.append(fold('c:' + cat.id, group.length > 0));
    const words = el('div', 'vol-settings-title');
    words.append(el('div', '', cat.title));
    if (managing && cat.description) {
      words.append(el('div', 'sub', cat.description));
    }
    name.append(words);
    if (folded('c:' + cat.id) && group.length) {
      name.append(tally(group));
    }
    if (managing) {
      const pencil = editPencil(`Rename ${cat.title}`);
      pencil.addEventListener('click', () => rename(words, cat.title, async next => {
        await act('activity-categories', cat.id, 'edit', {title: next, description: cat.description || '', image: cat.image || '', allowAdding: cat.allowAddingOwn || ''});
        await load();
      }));
      name.append(pencil);
    }
    if (node.canEdit) {
      const plus = button('', 'plus', 'edit-icon', () => addUnder(heading, node, cat.id, 2));
      plus.title = `Add an activity to ${cat.title}`;
      plus.setAttribute('aria-label', plus.title);
      name.append(plus);
    }
    if (managing && root.canEdit) {
      const item = {id: cat.id, kind: 'category', index: i};
      draggable(heading, name, item);
      dropTarget(heading, item, d => d.kind === 'category' || d.kind === 'activity', async moved => {
        if (moved.kind === 'activity') {
          await place(moved, node, cat.id);
          return;
        }
        await moveCategory(node.id, cats, moved.index, moved.index < i ? i - 1 : i, null);
      }, false);
    }
    const locked = root.canEdit ? '' : 'Only the event’s organizers can change this';
    heading.classList.toggle('is-hidden', Boolean(cat.hidden) || root.status === 'Hidden');
    heading.append(
      eye(!cat.hidden, root.status === 'Hidden' ? root.title : '', locked, open => saveCategory(cat, {hidden: open ? '' : 'Yes'})),
      name,
      choice([{label: 'Default', value: ''}, {label: 'Unlimited', value: 'Yes'}, {label: 'No More', value: 'No'}], cat.directSignUpOwn || '', locked, v => saveCategory(cat, {directSignUp: v}), yesNo(root.directSignUp)),
      choice(visibility(), flip(cat.volunteersHiddenOwn), locked, v => saveCategory(cat, {volunteersHidden: flip(v)}), yesNo(!root.volunteersHidden)),
      choice(addingChoices(), cat.allowAddingOwn || '', locked, v => saveCategory(cat, {allowAdding: v}), root.allowAdding || ADDING.no),
    );
    list.append(heading);
    if (folded('c:' + cat.id)) {
      return;
    }
    for (const c of group) {
      row(c, 2, true, node);
    }
  });
  const loose = node.children.filter(c => !cats.some(cat => cat.id === (c.category || '')));
  if (open && loose.length) {
    const hidden = cats.length && folded('c:');
    if (cats.length) {
      const heading = el('div', 'vol-settings-row vol-settings-group is-block is-nested');
      const name = el('span', 'vol-settings-name');
      name.append(fold('c:', true), el('span', 'vol-settings-title', 'Uncategorized'));
      if (hidden) {
        name.append(tally(loose));
      }
      heading.append(el('span'), name, el('span'), el('span'), el('span'));
      list.append(heading);
    }
    if (!hidden) {
      for (const c of loose) {
        row(c, cats.length ? 2 : 1, true, node);
      }
    }
  }
  openModal(`${node.title}: Volunteer settings`, [el('p', 'vol-settings-lead', 'Configure sign up and visibility settings for activities and categories. Activities and categories can override its parent’s settings, or default to them.'), list], {wide: 'table', replace});
}

export async function openVolunteerGrid(root, nodes, pathOf) {
  await listed();
  const signUps = [];
  for (const node of nodes) {
    for (const v of node.volunteers) {
      signUps.push({node, v});
    }
  }
  const firstName = v => (v.name || v.email).split(' ')[0];
  const follows = {Parents: ['parents', 'Parent'], Children: ['children', 'Child'], Siblings: ['siblings', 'Sibling']};
  const familyOf = r => {
    const info = known(r.v.email);
    if (!info) {
      return [];
    }
    const out = [];
    for (const [relation, [path, as]] of Object.entries(follows)) {
      if (!relations.has(relation)) {
        continue;
      }
      for (const p of relativesOf(info, path).filter(emailOf)) {
        out.push({node: r.node, v: {email: emailOf(p), name: p.name_show, position: `${as} of ${firstName(r.v)}`}});
      }
    }
    return out;
  };
  const relations = new Set();
  let ids = new Set(nodes.map(n => n.id));
  const titleOf = r => {
    const info = known(r.v.email);
    if (!info) {
      return '';
    }
    if (isStudent(info)) {
      return gradeOf(info) || 'Student';
    }
    return [isParent(info) ? 'Parent' : '', isStaff(info) ? 'Staff' : ''].filter(Boolean).join(', ');
  };
  const columns = [
    {label: 'Volunteer', get: r => r.v.name || r.v.email},
    {label: 'Title', get: titleOf},
    {label: 'Where', get: r => pathOf(r.node)},
    {label: 'As', get: r => r.v.position},
    {label: 'Sign Up Date', get: r => (r.v.added ? longDate(r.v.added) : ''), sort: r => r.v.added || ''},
    {label: 'Email', get: r => r.v.email, show: r => mailto(r.v.email)},
  ];
  const holder = el('div');
  const count = el('span', 'roster-count');
  const paint = () => {
    const seen = new Set(signUps.map(r => r.v.email));
    const rows = [...signUps];
    for (const r of signUps) {
      for (const k of familyOf(r)) {
        if (!seen.has(k.v.email)) {
          seen.add(k.v.email);
          rows.push(k);
        }
      }
    }
    const grid = dataGrid({columns, rows});
    grid.show(r => ids.has(r.node.id));
    holder.replaceChildren(rows.length ? grid.wrap : el('div', 'panel-empty', 'Nobody has signed up yet.'));
    const n = grid.shown().length;
    count.textContent = `${n} ${n === 1 ? 'person' : 'people'}`;
  };
  const tools = el('div', 'roster-tools');
  tools.append(count);
  const buttons = el('div', 'roster-buttons');
  buttons.append(familyDropdown(['Parents', 'Children', 'Siblings'], relations, paint));
  if (nodes.length > 1) {
    const filter = treeFilter(root, nodes.slice(1), sources => {
      ids = new Set(sources.map(n => n.id));
      paint();
    }, nodes.map(n => n.id));
    buttons.append(filter.wrap);
  }
  tools.append(buttons);
  paint();
  openModal(`${root.title}: Volunteers`, [tools, holder], {wide: true});
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
  const fields = [titleField, field('Description', description), image.wrap];
  if (!eventId) {
    fields.push(addingField);
  }
  const onMain = eventId ? null : checkbox('Show on the main page (it stays in the toolbar either way)', category ? category.showOnMain : true);
  if (onMain) {
    fields.push(onMain.wrap);
  }
  openModal(category ? 'Edit Category' : 'Add Category', fields, {
    replace: true,
    saveLabel: category ? 'Save changes' : 'Add',
    submit: () => {
      const fields = {
        title: title.value, description: description.value, image: image.value(), allowAdding: adding.value,
        showOnMain: onMain ? onMain.input.checked : true,
      };
      return category ? act('activity-categories', category.id, 'edit', fields) : create('activity-categories', {eventId: eventId || '', ...fields});
    },
    afterSave: after,
    onDelete: category && category.can.delete ? () => remove('activity-categories', category.id) : null,
    confirmDelete: category ? `Delete the category “${category.title}”?` : '',
    afterDelete: after,
  });
}

function addingWords(category) {
  const own = category.allowAddingOwn || '';
  const word = {[ADDING.yes]: 'People can add here', [ADDING.approval]: 'People can suggest here', [ADDING.no]: 'Only organizers add here'}[category.allowAdding] || '';
  return own ? word : (category.eventId ? `${word} (same as the event)` : word);
}

async function moveCategory(eventId, list, from, to, after) {
  const ids = list.map(c => c.id);
  const [moved] = ids.splice(from, 1);
  ids.splice(to, 0, moved);
  try {
    await (eventId ? act('activities', eventId, 'order-categories', {ids}) : act('team-settings', state.model.settingsId, 'order-categories', {ids}));
    await load();
    if (after) {
      after();
    }
  } catch (err) {
    toast(err.message);
  }
}

export function categoryList(root, after) {
  const wrap = el('div', 'category-list');
  const eventId = root ? root.id : '';
  const list = root ? eventCategories(root) : state.model.categories.filter(c => !c.builtIn);
  const move = (from, to) => moveCategory(eventId, list, from, to, after);
  list.forEach((category, i) => {
    const row = el('div', 'admin-row');
    if (category.imageUrl) {
      row.append(imageThumb(category.imageUrl, category.title, 'small'));
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

export function openSettings() {
  const settings = state.model.settings;
  const expense = text(settings.expenseFormUrl, {type: 'url', required: true});
  const intro = textarea(settings.intro, 4);
  openModal('Settings', [field('Expense form URL', expense), field('Intro', intro, 'Shown under the Sign Up heading')], {
    submit: () => act('team-settings', state.model.settingsId, 'settings', {expenseFormUrl: expense.value, intro: intro.value}),
  });
}

export function openRedirect(item) {
  const old = text(item ? item.old : '', {required: true, placeholder: 'https://hca.heliosian.com/dl/signup/... or /old/path'});
  const to = text(item ? item.new : '', {required: true, placeholder: '/v/spring-celebration, or https://...'});
  openModal(item ? 'Edit Redirect' : 'Add Redirect', [
    field('Old address', old, 'The link people still hold: paste the whole link, or its path. A bare word is a friendly address, /v/word.'),
    field('Send them to', to, 'A page here, as its path, or a whole address on another site. A link into what sits under the old address follows along.'),
  ], {
    submit: () => (item ? act('activity-redirects', item.id, 'edit', {old: old.value, new: to.value}) : create('activity-redirects', {old: old.value, new: to.value})),
    onDelete: item && item.can.delete ? () => remove('activity-redirects', item.id) : null,
    confirmDelete: item ? `Remove the redirect from ${item.old}? Anyone holding that link will get Not found.` : '',
    deleteLabel: 'Remove',
  });
}

export async function copyToNextYear(node) {
  if (!confirm(`Copy “${node.title}” and everything under it into ${years().next}?`)) {
    return;
  }
  try {
    await act('activities', node.id, 'copy');
    await load();
    toast(`Copied to ${years().next}`);
  } catch (err) {
    toast(err.message);
  }
}

export async function saveActivityFields(act, changes) {
  try {
    await saveActivity({id: act.id, ...changes});
  } catch (err) {
    toast(err.message);
    await load();
  }
}

