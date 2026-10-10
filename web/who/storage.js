export function loadLastTag() {
  return localStorage.getItem('lastTag') || '';
}

export function saveLastTag(tag) {
  localStorage.setItem('lastTag', tag);
}

export function loadTagUsage() {
  return JSON.parse(localStorage.getItem('tagUsage') || '{}');
}

export function recordTagUsage(tag) {
  const usage = loadTagUsage();
  usage[tag] = Date.now();
  localStorage.setItem('tagUsage', JSON.stringify(usage));
}

export function loadTagRelations(tag) {
  const all = JSON.parse(localStorage.getItem('tagRelations') || '{}');
  return new Set(all[tag] || []);
}

export function saveTagRelations(tag, relations) {
  const all = JSON.parse(localStorage.getItem('tagRelations') || '{}');
  all[tag] = [...relations];
  localStorage.setItem('tagRelations', JSON.stringify(all));
}

export function loadNavScroll() {
  return Number(sessionStorage.getItem('navScroll')) || 0;
}

export function saveNavScroll(top) {
  sessionStorage.setItem('navScroll', String(top));
}
