export function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) {
    node.className = className;
  }
  if (text !== undefined) {
    node.textContent = text;
  }
  return node;
}

const paths = {
  go: 'M15 3h6v6M10 14 21 3M21 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5',
  copy: 'M8 8h11a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V10a2 2 0 0 1 2-2zM16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3',
  more: 'M5 12h.01M12 12h.01M19 12h.01',
  edit: 'M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z',
  home: 'm3 10 9-7 9 7v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z',
  section: 'M4 5h6v6H4zM14 5h6v6h-6zM4 15h6v6H4zM14 15h6v6h-6z',
  school: 'M3 21h18M5 21V9l7-5 7 5v12M10 21v-5h4v5M9 12h.01M15 12h.01M12 8h.01',
  calendar: 'M4 5h16a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM3 10h18M8 3v4M16 3v4',
  star: 'M12 3l2.8 5.7 6.2.9-4.5 4.4 1.1 6.2L12 17.3 6.4 20.2l1.1-6.2L3 9.6l6.2-.9z',
  chat: 'M21 12a8 8 0 0 1-11.6 7.2L4 21l1.8-4.4A8 8 0 1 1 21 12z',
  plus: 'M12 5v14M5 12h14',
  // The rule editor's marks (rules.js), as Loop draws them.
  groups: 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM22 21v-2a4 4 0 0 0-3-3.9M16 3.1a4 4 0 0 1 0 7.8',
  'user-minus': 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM23 11h-6',
  trash: 'M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3',
  tag: 'M20.6 13.4 13.4 20.6a2 2 0 0 1-2.8 0L2 12V2h10l8.6 8.6a2 2 0 0 1 0 2.8zM7 7h.01',
  families: 'M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM23 21v-2a4 4 0 0 0-3-3.9M16 3.1a4 4 0 0 1 0 7.8',
  party: 'M5.8 11.3 2 22l10.7-3.8M4 3h.01M22 8h.01M15 2h.01M22 20h.01M22 2l-2.2 2.2M15.7 3.7 14 5.4M18 8.3l-1.8 1.8M20 13.5l-1.5 1.5M9 5.5 3.5 11',
  activity: 'M18 8h1a4 4 0 0 1 0 8h-1M2 8h16v9a4 4 0 0 1-4 4H6a4 4 0 0 1-4-4V8zM6 1v3M10 1v3M14 1v3',
  classrooms: 'M3 21h18M5 21V7l8-4v18M19 21V11l-6-4M9 9h.01M9 13h.01M9 17h.01',
  calendarAdd: 'M4 5h16a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM3 9.5h18M8 3v3.5M16 3v3.5M12 12v7.5M8.25 15.75h7.5',
  volunteer: 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8M22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75',
  ticket: 'M3 9V6h18v3a2 2 0 0 0 0 6v3H3v-3a2 2 0 0 0 0-6zM13 6v12',
  check: 'M5 12l5 5L20 7',
  clock: 'M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18zM12 7v5l3 2',
  close: 'M6 6l12 12M18 6L6 18',
  chevron: 'm9 6 6 6-6 6',
  arrow: 'M5 12h14M13 6l6 6-6 6',
  family: 'M9 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM17 10a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5zM3 20v-1a5 5 0 0 1 5-5h2a5 5 0 0 1 5 5v1M15.5 14H16a4 4 0 0 1 4 4v2',
  // The marks a category can go by (categoryIcons), drawn in one line
  // like the rest: Heliosian's own petals and sun, a pushpin, a link, a
  // map pin, and the everyday things a section holds.
  heliosian: 'M6 3h3a3 3 0 0 1 3 3v3a3 3 0 0 1-3 3H6a3 3 0 0 1-3-3V6a3 3 0 0 1 3-3zM15 3h3a3 3 0 0 1 3 3v3a3 3 0 0 1-3 3h-3a3 3 0 0 1-3-3V6a3 3 0 0 1 3-3zM6 12h3a3 3 0 0 1 3 3v3a3 3 0 0 1-3 3H6a3 3 0 0 1-3-3v-3a3 3 0 0 1 3-3zM15 12h3a3 3 0 0 1 3 3v3a3 3 0 0 1-3 3h-3a3 3 0 0 1-3-3v-3a3 3 0 0 1 3-3zM12 8.5a3.5 3.5 0 1 1 0 7 3.5 3.5 0 0 1 0-7z',
  pin: 'M9 3h6l-1 6 3 3v2H7v-2l3-3-1-6zM12 14v7',
  link: 'M10 14a4.5 4.5 0 0 0 6.4 0l2.2-2.2a4.5 4.5 0 0 0-6.4-6.4l-1.1 1.1M14 10a4.5 4.5 0 0 0-6.4 0l-2.2 2.2a4.5 4.5 0 0 0 6.4 6.4l1.1-1.1',
  map: 'M12 21s-6-5.3-6-11a6 6 0 0 1 12 0c0 5.7-6 11-6 11zM12 12.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z',
  heart: 'M12 21s-8-5.5-8-11.5A4.5 4.5 0 0 1 12 7a4.5 4.5 0 0 1 8 2.5C20 15.5 12 21 12 21z',
  book: 'M4 5a2 2 0 0 1 2-2h13v16H6a2 2 0 0 0-2 2zM4 5v16M6 19h13',
  music: 'M9 18V6l10-2v12M9 18a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0zM19 16a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0z',
  sun: 'M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10zM12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4',
  camera: 'M4 8h3l2-3h6l2 3h3v11H4zM12 17a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7z',
  gift: 'M3 9h18v3H3zM5 12v9h14v-9M12 9v12M12 9c-2-4-6-4-6-1s4 1 6 1zM12 9c2-4 6-4 6-1s-4 1-6 1z',
  ball: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM3.5 10.5l5-1 2.5-4M20.5 10.5l-5-1-2.5-4M12 9.5l3.5 2.5-1.3 4h-4.4L8.5 12z',
  bell: 'M6 16V11a6 6 0 0 1 12 0v5l2 2H4zM10 21h4',
  cart: 'M3 4h2l2.5 11h11L21 7H6.5M9 20a1 1 0 1 0 0-2 1 1 0 0 0 0 2zM17 20a1 1 0 1 0 0-2 1 1 0 0 0 0 2z',
  bulb: 'M9 18h6M10 21h4M12 3a6 6 0 0 0-4 10.5c.7.7 1 1.5 1 2.5h6c0-1 .3-1.8 1-2.5A6 6 0 0 0 12 3z',
  megaphone: 'M3 10v4h3l7 4V6l-7 4H3zM17 9a4 4 0 0 1 0 6',
  hand: 'M8 13V5a1.5 1.5 0 0 1 3 0v6M11 11V4a1.5 1.5 0 0 1 3 0v7M14 11V5.5a1.5 1.5 0 0 1 3 0V13M17 12a1.5 1.5 0 0 1 3 1v2a6 6 0 0 1-6 6h-2a6 6 0 0 1-5-2.7L4 14a1.5 1.5 0 0 1 2.4-1.8L8 14',
};

// maskedIcons are the marks drawn from a picture rather than a path
// (web/home/brand/symbol-<name>.png, and style.css's .icon-<name>).
const maskedIcons = ['heliosian', 'when'];

// categoryIcons are the marks the category editor offers, in the order it
// shows them, each with the words under it.
export const categoryIcons = [
  ['heliosian', 'Heliosian'], ['when', 'When'], ['pin', 'Pin'], ['link', 'Link'], ['calendar', 'Event'], ['chat', 'Chat'],
  ['school', 'School'], ['volunteer', 'People'], ['family', 'Family'], ['star', 'Star'], ['heart', 'Heart'],
  ['book', 'Book'], ['music', 'Music'], ['ball', 'Sports'], ['ticket', 'Ticket'], ['gift', 'Gift'],
  ['camera', 'Photos'], ['sun', 'Sun'], ['map', 'Place'], ['bell', 'Bell'], ['cart', 'Shop'],
  ['bulb', 'Idea'], ['megaphone', 'News'], ['hand', 'Help'], ['home', 'Home'], ['section', 'Grid'],
];

// iconOf reads a category's mark: "icon:<name>" in the sheet's Emoji cell
// names one of the marks above; anything else (an emoji from before) is
// read as none, and the mark comes off the title instead.
export function iconOf(category) {
  const value = category.emoji || '';
  if (value.startsWith('icon:') && (paths[value.slice(5)] || maskedIcons.includes(value.slice(5)))) {
    return value.slice(5);
  }
  return categoryIcon(category.title);
}

// The glyph a category goes by, in the rail and at the head of its section:
// read off its title, since the sheet's image is a picture rather than an
// outline - a school building, a calendar, a chat bubble, else a plain grid.
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

export function svg(name) {
  // The apps' own marks are the designer's white outlines (each app's
  // symbol_white export), drawn through a mask so they take the text
  // colour like the rest.
  if (maskedIcons.includes(name)) {
    const node = document.createElement('span');
    node.className = 'icon-mask icon-' + name;
    node.setAttribute('aria-hidden', 'true');
    return node;
  }
  const node = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  node.setAttribute('viewBox', '0 0 24 24');
  node.setAttribute('fill', 'none');
  node.setAttribute('stroke', 'currentColor');
  node.setAttribute('stroke-width', '2');
  node.setAttribute('stroke-linecap', 'round');
  node.setAttribute('stroke-linejoin', 'round');
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  path.setAttribute('d', paths[name]);
  node.append(path);
  return node;
}

export function displayURL(url) {
  return url.replace(/^https?:\/\//, '').replace(/\/$/, '');
}

let toastTimer;

export function toast(message) {
  const node = document.querySelector('#toast');
  node.textContent = message;
  node.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    node.hidden = true;
  }, 1800);
}
