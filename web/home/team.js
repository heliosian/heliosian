import {rowsOf} from '/rail.js';

export const teamQueries = {
  activities: '(from GROUP (where (= kind "activity") (!= status "closed")) (order order asc) (columns kind name slug parent status start end timing capacity join priority visible_to managed_by leads team_href))',
  headings: '(from GROUP (where (= kind "category") (= parent.kind "activity")) (columns kind parent visible_to))',
  everyone: '(from GROUP (where (= id (slug "everyone"))) (columns slug))',
  mine: '(from MEMBER (where (= person @viewer) (= member "yes") (in group (select GROUP.id (= kind "activity")))) (columns group))',
  led: '(from MEMBER (where (= person @viewer) (= member "yes") (in group (select GROUP.leads (= kind "activity")))) (columns group))',
  chairs: '(from MEMBER (where (= person @viewer) (= member "yes") (in group (select GROUP.managed_by (= kind "activity")))) (columns group))',
  taken: '(from MEMBER (where (= member "yes") (in group (select GROUP.id (= kind "activity") (not (blank capacity))))) (columns group))',
  teamPhotos: '(from PHOTO (where (in group (select GROUP.id (= kind "activity")))) (order order asc) (columns group))',
};

const neededCount = 12;

function schoolYear(today) {
  const [y, m] = today.split('-').map(Number);
  const start = m >= 7 ? y : y - 1;
  return `${start} - ${start + 1}`;
}

function dayOf(cell) {
  return (cell || '').slice(0, 10);
}

export function teamWidget(data, now) {
  const today = now.slice(0, 10);
  const year = schoolYear(today);
  const everyone = data.everyone.result[0] || '';
  const groups = new Map();
  for (const g of [...rowsOf(data.activities, 'GROUP'), ...rowsOf(data.headings, 'GROUP')]) {
    groups.set(g.id, g);
  }
  const isYear = g => g.kind === 'activity' && !groups.has(g.parent);
  const up = g => {
    let p = groups.get(g.parent);
    while (p && p.kind === 'category') {
      p = groups.get(p.parent);
    }
    return p || null;
  };
  const above = g => {
    const out = [];
    for (let p = up(g); p && !isYear(p); p = up(p)) {
      out.push(p);
    }
    return out;
  };
  const yearOf = g => {
    for (let p = up(g); p; p = up(p)) {
      if (isYear(p)) {
        return p.name;
      }
    }
    return '';
  };
  const shown = g => {
    for (let n = g; n; n = groups.get(n.parent)) {
      if (n.visible_to !== everyone || n.status === 'pending') {
        return false;
      }
    }
    return true;
  };
  const taken = {};
  for (const m of rowsOf(data.taken, 'MEMBER')) {
    taken[m.group] = (taken[m.group] || 0) + 1;
  }
  const picture = {};
  for (const p of rowsOf(data.teamPhotos, 'PHOTO')) {
    picture[p.group] = picture[p.group] || p.id;
  }
  const on = {};
  for (const m of rowsOf(data.mine, 'MEMBER')) {
    on[m.group] = 'Volunteer';
  }
  const chairing = new Set([...rowsOf(data.led, 'MEMBER'), ...rowsOf(data.chairs, 'MEMBER')].map(m => m.group));
  for (const g of groups.values()) {
    if (on[g.id] && (chairing.has(g.leads) || chairing.has(g.managed_by))) {
      on[g.id] = 'Co-Chair';
    }
  }
  const children = {};
  for (const g of groups.values()) {
    const p = g.kind === 'activity' && !isYear(g) ? up(g) : null;
    if (p) {
      (children[p.id] = children[p.id] || []).push(g);
    }
  }
  const below = g => (children[g.id] || []).flatMap(c => [c, ...below(c)]);
  const activities = [...groups.values()].filter(isYear).flatMap(below);
  const timed = g => [g, ...above(g)].find(n => n.start || n.timing) || g;
  const lastDay = g => dayOf(g.end || g.start);
  const spots = g => Number(g.capacity) || 0;
  const full = g => g.join === 'none' || (spots(g) > 0 && (taken[g.id] || 0) >= spots(g));
  const wanted = g => {
    if (yearOf(g) !== year || g.status !== 'open' || !shown(g) || full(g)) {
      return false;
    }
    const last = lastDay(timed(g));
    return !last || last >= today;
  };
  const wants = g => {
    let label = '';
    let needed = 0;
    for (const n of [g, ...below(g)]) {
      if (n.status !== 'open' || n.join === 'none' || !shown(n)) {
        continue;
      }
      const left = spots(n) - (taken[n.id] || 0);
      if (spots(n) > 0 && left > 0) {
        needed += left;
        if (!label && n !== g) {
          label = n.name;
        }
      }
    }
    const parts = label ? [label] : [];
    if (needed === 1) {
      parts.push('1 volunteer needed');
    } else if (needed > 1) {
      parts.push(`${needed} volunteers needed`);
    } else if (!label) {
      parts.push('Volunteers wanted');
    }
    return parts.join(' · ');
  };
  const item = g => {
    const when = timed(g);
    const pic = [g, ...above(g)].map(n => picture[n.id]).find(Boolean);
    return {
      title: g.name, under: above(g).reverse().map(n => n.name).join(' › '), start: dayOf(when.start), timing: when.timing || '',
      position: on[g.id] || '', note: g.status === 'open' ? wants(g) : '', href: g.team_href,
      image: pic ? `/api/blob/${pic}/image` : '',
    };
  };
  const byDay = (x, y) => {
    const dx = dayOf(timed(x).start);
    const dy = dayOf(timed(y).start);
    if (!dx !== !dy) {
      return dx ? -1 : 1;
    }
    return dx.localeCompare(dy);
  };
  const mine = activities.filter(g => on[g.id] && yearOf(g) === year && g.status !== 'done' && (!lastDay(timed(g)) || lastDay(timed(g)) >= today)).sort(byDay);
  const priority = activities.filter(g => g.priority === 'Yes' && wanted(g) && !on[g.id]).sort(byDay);
  const roots = activities.filter(g => isYear(up(g) || {}) && yearOf(g) === year && g.status === 'open' && shown(g) && !full(g));
  const dated = roots.filter(g => g.start && dayOf(g.start) >= today).sort((x, y) => x.start.localeCompare(y.start));
  const undated = roots.filter(g => !g.start);
  const open = [...dated, ...undated].filter(g => !on[g.id]).slice(0, neededCount);
  return {mine: mine.map(item), open: open.map(item), priority: priority.map(item)};
}
