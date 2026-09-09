export function loadNavOpen() {
  try {
    const raw = localStorage.getItem('navOpen');
    if (raw) {
      return JSON.parse(raw);
    }
  } catch (e) {
    // ignore, fall through to defaults
  }
  return {directory: true, family: false, tools: true};
}

export function saveNavOpen(navOpen) {
  try {
    localStorage.setItem('navOpen', JSON.stringify(navOpen));
  } catch (e) {
    // ignore
  }
}

// The last tag this user assigned to anyone, anywhere - remembered per
// browser so a click on the tag button can immediately reapply it instead of
// making every tagging start from an empty dropdown.
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
    // ignore
  }
}

// When each tag was last applied to someone, remembered per browser so the
// tag picker's checkbox list can surface the tags this user actually reaches
// for instead of an alphabetical list that never changes.
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
    // ignore
  }
}

// Which relations (Parents/Children/Siblings) to pull into each tag's list,
// remembered per tag name so "Birthday" and "Carpool" can each keep their
// own Include settings across visits.
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
    // ignore
  }
}
