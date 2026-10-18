import {groupAt, groupKeyOf, kindAt} from './state.js';
import {el, svg, link} from '/elements.js';
import {trail} from '/router.js';
import {tagsOf, tagControl, tagLabel, tagHref} from './tags.js';
import {setChrome} from './chrome.js';

export function fromURL() {
  const raw = trail()[0];
  if (!raw) {
    return null;
  }
  return new URL(raw, location.origin);
}

function segmentsOf(href) {
  return new URL(href, location.origin).pathname.split('/').filter(Boolean).map(decodeURIComponent);
}

function classroomsBack() {
  const parent = trail()[1];
  const kind = parent ? kindAt(segmentsOf(parent)) : '';
  return parent && (parent.startsWith('/classrooms') || kind === 'classroom' || kind === 'grade') ? parent : '/classrooms';
}

export function peopleCrumbs(from) {
  const back = from.pathname + from.search;
  const seg = segmentsOf(from.pathname);
  const key = kindAt(seg) === 'list' ? groupKeyOf(seg[1]) : '';
  if (!key) {
    return [['People', back]];
  }
  return [['People', '/people'], [tagLabel(key), back]];
}

export function listedFrom(href) {
  if (!href) {
    return false;
  }
  const seg = segmentsOf(href);
  return (seg[0] === 'people' && !seg[1]) || kindAt(seg) === 'list';
}

export function fromCrumbs() {
  const from = fromURL();
  if (!from) {
    return null;
  }
  const seg = segmentsOf(from.pathname);
  const back = from.pathname + from.search;
  const kind = kindAt(seg);
  if (kind === 'grade' || kind === 'classroom') {
    return [['Gradebands', classroomsBack()], [groupAt(seg[1]).name, back]];
  }
  if (seg[0] === 'people' || kind === 'list') {
    return peopleCrumbs(from);
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

export function breadcrumbs(parts, tagPerson, action) {
  const parent = [...parts].reverse().find(([, href]) => href);
  setChrome(parts[parts.length - 1][0], parent ? parent[1] : '/people');
  const top = el('div', 'detail-top container');
  const crumbs = el('div', 'crumbs');
  const back = link(parent ? parent[1] : '/people', 'crumb-back');
  back.append(svg('chevron-left'));
  crumbs.append(back);
  parts.forEach(([label, href], i) => {
    if (i > 0) {
      crumbs.append(el('span', 'crumb-sep', '/'));
    }
    if (href) {
      crumbs.append(link(href, 'crumb', label));
    } else {
      crumbs.append(el('span', 'crumb current', label));
    }
  });
  top.append(crumbs);
  const right = el('div', 'detail-top-right');
  if (tagPerson) {
    const tagArea = el('div', 'tag-area');
    const tagList = el('div', 'tag-list');
    const renderTagList = () => {
      tagList.replaceChildren();
      for (const key of tagsOf(tagPerson)) {
        const chip = link(tagHref(key), 'tag-chip', tagLabel(key));
        chip.title = `See everyone tagged "${tagLabel(key)}"`;
        tagList.append(chip);
      }
    };
    renderTagList();
    const tagWrap = tagControl(tagPerson, 'tag-wrap', 'tag-button', renderTagList);
    tagWrap.querySelector('.tag-button').title = 'Tags (Shift+T)';
    tagArea.append(tagList, tagWrap);
    right.append(tagArea);
  }
  if (action) {
    right.append(action);
  }
  if (right.children.length) {
    top.append(right);
  }
  return top;
}
