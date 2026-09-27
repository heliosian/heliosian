export function loadNavOpen() {
  try {
    const raw = localStorage.getItem('navOpen');
    if (raw) {
      return JSON.parse(raw);
    }
  } catch (e) {
  }
  return {directory: true, family: false, tools: true};
}

export function saveNavOpen(navOpen) {
  try {
    localStorage.setItem('navOpen', JSON.stringify(navOpen));
  } catch (e) {
  }
}

export function loadLastTag() {
  try {
    return localStorage.getItem('lastTag') || '';
  } catch (e) {
    return '';
  }
}

export function saveLastTag(tag) {
  try {
    localStorage.setItem('lastTag', tag);
  } catch (e) {
  }
}

export function loadTagUsage() {
  try {
    return JSON.parse(localStorage.getItem('tagUsage') || '{}');
  } catch (e) {
    return {};
  }
}

export function recordTagUsage(tag) {
  try {
    const usage = loadTagUsage();
    usage[tag] = Date.now();
    localStorage.setItem('tagUsage', JSON.stringify(usage));
  } catch (e) {
  }
}

export function loadTagRelations(tag) {
  try {
    const all = JSON.parse(localStorage.getItem('tagRelations') || '{}');
    return new Set(all[tag] || []);
  } catch (e) {
    return new Set();
  }
}

export function saveTagRelations(tag, relations) {
  try {
    const all = JSON.parse(localStorage.getItem('tagRelations') || '{}');
    all[tag] = [...relations];
    localStorage.setItem('tagRelations', JSON.stringify(all));
  } catch (e) {
  }
}

export function loadNavScroll() {
  return Number(sessionStorage.getItem('navScroll')) || 0;
}

export function saveNavScroll(top) {
  sessionStorage.setItem('navScroll', String(top));
}
