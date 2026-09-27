import {el, svg} from '/elements.js';

const maskedIcons = ['heliosian', 'when'];

export const categoryIcons = [
  ['heliosian', 'Heliosian'], ['when', 'When'], ['pin', 'Pin'], ['link', 'Link'], ['calendar', 'Event'], ['chat', 'Chat'],
  ['school', 'School'], ['volunteer', 'People'], ['family', 'Family'], ['star', 'Star'], ['heart', 'Heart'],
  ['book', 'Book'], ['music', 'Music'], ['ball', 'Sports'], ['ticket', 'Ticket'], ['gift', 'Gift'],
  ['camera', 'Photos'], ['sun', 'Sun'], ['map', 'Place'], ['bell', 'Bell'], ['cart', 'Shop'],
  ['bulb', 'Idea'], ['megaphone', 'News'], ['hand', 'Help'], ['home', 'Home'], ['section', 'Grid'],
];

export function iconOf(category) {
  const value = category.emoji || '';
  if (value.startsWith('icon:') && categoryIcons.some(([name]) => name === value.slice(5))) {
    return value.slice(5);
  }
  return categoryIcon(category.title);
}

export function categoryIcon(title) {
  const t = title.toLowerCase();
  if (/school|class|campus/.test(t)) {
    return 'school';
  }
  if (/event|calendar|date/.test(t)) {
    return 'calendar';
  }
  if (/chat|group|talk|message/.test(t)) {
    return 'chat';
  }
  return 'section';
}

export function categoryMark(name) {
  if (!maskedIcons.includes(name)) {
    return svg(name);
  }
  const node = el('span', 'icon-mask icon-' + name);
  node.setAttribute('aria-hidden', 'true');
  return node;
}

export function displayURL(url) {
  return url.replace(/^https?:\/\//, '').replace(/\/$/, '');
}
