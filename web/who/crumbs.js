import {state} from './state.js';
import {el, svg, slugify} from './dom.js';
import {tagsOf, tagControl} from './tags.js';
import {setChrome} from './chrome.js';

export function fromURL() {
  const raw = new URLSearchParams(location.search).get('from');
  if (!raw) {
    return null;
  }
  return new URL(raw, location.origin);
}

function classroomsBackOf(from) {
  const parent = new URLSearchParams(from.search).get('from');
  return parent && parent.startsWith('/classrooms') ? parent : '/classrooms';
}

export function fromCrumbs() {
  const from = fromURL();
  if (!from) {
    return null;
  }
  const seg = from.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  const back = from.pathname + from.search;
  if (seg[0] === 'grades' && seg[1]) {
    const grade = state.model.grades.find(g => slugify(g.name) === seg[1]);
    if (grade) {
      return [['Gradebands', classroomsBackOf(from)], [grade.name, back]];
    }
  }
  if (seg[0] === 'classrooms' && seg[1]) {
    const classroom = state.model.classrooms.find(c => slugify(c.name) === seg[1]);
    if (classroom) {
      return [['Gradebands', classroomsBackOf(from)], [classroom.name, back]];
    }
  }
  if (seg[0] === 'people' && !seg[1]) {
    const tag = new URLSearchParams(from.search).get('tag');
    return [[tag || 'People', back]];
  }
  if (seg[0] === 'classrooms') {
    return [['Gradebands', back]];
  }
  if (seg[0] === 'staff') {
    return [['Staff', back]];
  }
  if (seg[0] === 'email-list') {
    return [['Everyone', back]];
  }
  return null;
}

export function breadcrumbs(parts, tagEmail) {
  const parent = [...parts].reverse().find(([, href]) => href);
  setChrome(parts[parts.length - 1][0], parent ? parent[1] : '/people');
  const top = el('div', 'detail-top container');
  const crumbs = el('div', 'crumbs');
  const back = el('a', 'crumb-back');
  back.href = parent ? parent[1] : '/people';
  back.append(svg('chevron-left'));
  crumbs.append(back);
  parts.forEach(([label, href], i) => {
    if (i > 0) {
      crumbs.append(el('span', 'crumb-sep', '/'));
    }
    if (href) {
      const a = el('a', 'crumb', label);
      a.href = href;
      crumbs.append(a);
    } else {
      crumbs.append(el('span', 'crumb current', label));
    }
  });
  top.append(crumbs);
  if (tagEmail) {
    const tagArea = el('div', 'tag-area');
    const tagList = el('div', 'tag-list');
    const renderTagList = () => {
      tagList.replaceChildren();
      for (const name of tagsOf(tagEmail)) {
        const chip = el('a', 'tag-chip', name);
        chip.href = '/people?tag=' + encodeURIComponent(name);
        chip.title = `See everyone tagged "${name}"`;
        tagList.append(chip);
      }
    };
    renderTagList();
    const tagWrap = tagControl(tagEmail, 'tag-wrap', 'tag-button', renderTagList);
    tagWrap.querySelector('.tag-button').title = 'Tags (Shift+T)';
    tagArea.append(tagList, tagWrap);
    top.append(tagArea);
  }
  return top;
}
