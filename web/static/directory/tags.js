import {tags} from './state.js';
import {loadLastTag, saveLastTag, loadTagUsage, recordTagUsage} from './storage.js';
import {el, svg} from './dom.js';

export function tagNames() {
  return Object.keys(tags).sort((a, b) => a.localeCompare(b));
}

// Same set of tags as tagNames(), ordered most-recently-used first (falling
// back to alphabetical for tags this browser has no usage record for, e.g.
// after clearing localStorage or on another device) - this is the order the
// tag picker's checkbox list shows, so the tags someone actually uses rise to
// the top instead of sitting wherever the alphabet puts them.
function tagNamesByRecency() {
  const usage = loadTagUsage();
  return tagNames().sort((a, b) => (usage[b] || 0) - (usage[a] || 0));
}

export function tagsOf(email) {
  return tagNames().filter(name => tags[name].includes(email));
}

function isTagged(email) {
  return tagNames().some(name => tags[name].includes(email));
}

async function setTag(email, tag, on) {
  const people = tags[tag] || [];
  if (on) {
    tags[tag] = people.includes(email) ? people : [...people, email];
    saveLastTag(tag);
    recordTagUsage(tag);
  } else {
    tags[tag] = people.filter(e => e !== email);
    if (!tags[tag].length) {
      delete tags[tag];
    }
  }
  const form = new FormData();
  form.append('person', email);
  form.append('tag', tag);
  form.append('on', on ? '1' : '0');
  const res = await fetch('/api/directory/tag', {method: 'POST', body: form});
  if (!res.ok) {
    alert(await res.text());
  }
}

function tagMenu(email, onChange) {
  const menu = el('div', 'card-menu tag-menu');
  menu.hidden = true;
  // render() rebuilds every checkbox from scratch on each toggle (simplest way to
  // stay in sync with tagNamesByRecency() gaining/losing/reordering entries), which would otherwise
  // drop keyboard focus back to nothing on every Space press - focusTag puts it
  // back on the same tag's (new) checkbox so arrow keys/Space can keep going.
  menu.focusTag = name => {
    menu.querySelector(`.tag-option input[data-tag-name="${CSS.escape(name)}"]`)?.focus();
  };
  const render = () => {
    menu.replaceChildren();
    for (const name of tagNamesByRecency()) {
      const row = el('label', 'tag-option');
      const box = el('input');
      box.type = 'checkbox';
      box.dataset.tagName = name;
      box.checked = tags[name].includes(email);
      box.addEventListener('change', async () => {
        await setTag(email, name, box.checked);
        render();
        onChange();
        menu.focusTag(name);
      });
      row.append(el('span', '', name), box);
      menu.append(row);
    }
    const form = el('form', 'tag-new');
    const input = el('input');
    input.placeholder = 'New tag';
    input.maxLength = 40;
    form.append(input);
    form.addEventListener('submit', async e => {
      e.preventDefault();
      const name = input.value.trim();
      if (!name) {
        return;
      }
      input.value = '';
      await setTag(email, name, true);
      render();
      onChange();
    });
    menu.append(form);
  };
  render();
  // The menu lives inside the card's own <a>, so a plain click here would
  // otherwise bubble up to (or, for a non-self-activating target like the
  // "New tag" input, resolve straight to) the card's link and navigate to
  // the profile page. Checkboxes and their labels already shield themselves
  // from that - they're self-activating - and must keep working natively
  // (preventDefault on them would cancel their own toggle too), so this only
  // steps in for everything else in the menu.
  menu.addEventListener('click', e => {
    e.stopPropagation();
    if (!e.target.closest('.tag-option')) {
      e.preventDefault();
    }
  });
  // Checkboxes only take Tab natively - Up/Down lets a keyboard user walk the
  // list the same way the global search dropdown's results do, so reaching a
  // tag to toggle off never requires the mouse.
  menu.addEventListener('keydown', e => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') {
      return;
    }
    const boxes = [...menu.querySelectorAll('.tag-option input[type="checkbox"]')];
    const current = boxes.indexOf(document.activeElement);
    if (current === -1) {
      return;
    }
    e.preventDefault();
    const delta = e.key === 'ArrowDown' ? 1 : -1;
    boxes[(current + delta + boxes.length) % boxes.length].focus();
  });
  menu.refreshTags = render;
  return menu;
}

// The tag to quick-assign on a click: the last one used, but only if it's
// still one of the user's actual current tags - a remembered name whose last
// person got untagged is gone from tagNames() even though localStorage still
// has it, and reapplying it would resurrect a tag the user no longer has.
// With no current tags at all, "My List" is the starting point.
function mostRecentTag() {
  const existing = tagNames();
  if (!existing.length) {
    return 'My List';
  }
  const last = loadLastTag();
  return existing.includes(last) ? last : existing[0];
}

export function tagControl(email, wrapClass, buttonClass, onChange) {
  const wrap = el('div', wrapClass);
  const button = el('button', buttonClass + (isTagged(email) ? ' active' : ''));
  button.title = 'Tags';
  button.append(svg('tag'));
  const menu = tagMenu(email, () => {
    button.classList.toggle('active', isTagged(email));
    onChange();
  });
  button.addEventListener('click', async e => {
    e.preventDefault();
    e.stopPropagation();
    const opening = menu.hidden;
    menu.hidden = !menu.hidden;
    if (!opening) {
      return;
    }
    if (isTagged(email)) {
      // Already has at least one tag - just show them to review or adjust,
      // rather than guessing another one on top.
      menu.querySelector('.tag-option input')?.focus();
      return;
    }
    // Opening the dropdown for someone with no tags yet applies the user's
    // most recently used tag first (creating "My List" the very first time),
    // so tagging someone new is a single click in the common case; the
    // dropdown that comes up right after still shows every tag as a checkbox
    // to adjust or undo the guess.
    const tag = mostRecentTag();
    await setTag(email, tag, true);
    menu.refreshTags();
    button.classList.toggle('active', isTagged(email));
    onChange();
    // Land keyboard focus straight on the tag that was just applied, so a
    // keyboard user's very next keystroke - Space - undoes it without first
    // hunting for it via Tab or the arrow keys.
    menu.focusTag(tag);
  });
  // On a mouse-driven desktop, hovering the button previews the dropdown
  // without the click handler's "apply my most recent tag" side effect -
  // hover is just a look, a click is a commitment. The brief delay before
  // hiding survives the small visual gap between the button and the menu
  // below it, so crossing that gap doesn't flicker the menu shut. Skipped
  // entirely on touch, where there's no hover state to preview with.
  if (window.matchMedia('(hover: hover) and (pointer: fine)').matches) {
    let closeTimer = null;
    wrap.addEventListener('mouseenter', () => {
      clearTimeout(closeTimer);
      menu.hidden = false;
    });
    wrap.addEventListener('mouseleave', () => {
      closeTimer = setTimeout(() => {
        menu.hidden = true;
      }, 150);
    });
  }
  wrap.append(button, menu);
  return wrap;
}
