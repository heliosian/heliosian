import {rowsOf} from '/rail.js';
import {availabilityOf} from '/calendar.js';

const upcoming = '(= kind "party") (not (in status "closed" "pending")) (not (blank visible_to)) (not (blank start)) (or (>= start today) (>= end today))';

export const partyQueries = {
  parties: `(from GROUP (where ${upcoming}) (order start asc) (columns name slug start end status join capacity waitlist visible_to celebrate_href))`,
  partyTickets: `(from MEMBER (where (= member "yes") (or (in group (select GROUP.id ${upcoming})) (in group (select GROUP.waitlist ${upcoming})))) (columns group person guest_of added_by))`,
  partyViewer: '(from PERSON (where (= id @viewer)) (columns name_short))',
  partyHousehold: '(from PERSON (where (in id (household @viewer))) (columns name_short))',
  partyPhotos: `(from PHOTO (where ready (in group (select GROUP.id ${upcoming}))) (order order asc) (columns group))`,
};

const callWords = {available: 'Get tickets', waitlist: 'Join the waitlist', 'sold-out': 'Sold out'};

function joinNames(names) {
  if (names.length < 2) {
    return names[0] || '';
  }
  return names.slice(0, -1).join(', ') + ' and ' + names[names.length - 1];
}

function mineWords(mine, who) {
  const names = joinNames(who) === 'You' ? '' : joinNames(who);
  if (mine === 'waitlisted') {
    if (names) {
      return `${names} ${who.length > 1 ? 'are' : 'is'} waitlisted`;
    }
    return 'Waitlisted';
  }
  if (mine === 'going') {
    if (who.length > 1) {
      return names + ' have tickets';
    }
    if (names) {
      return names + ' has a ticket';
    }
    return 'You have a ticket';
  }
  return '';
}

export function partiesWidget(data) {
  const now = new Date();
  const viewer = data.partyViewer.result[0];
  const first = {};
  for (const p of [...rowsOf(data.partyHousehold, 'PERSON'), ...rowsOf(data.partyViewer, 'PERSON')]) {
    first[p.id] = p.id === viewer ? 'You' : p.name_short;
  }
  const holderOf = m => [m.person, m.guest_of, m.added_by].find(id => id && first[id]) || '';
  const members = {};
  for (const m of rowsOf(data.partyTickets, 'MEMBER')) {
    (members[m.group] = members[m.group] || []).push(m);
  }
  const picture = {};
  for (const ph of rowsOf(data.partyPhotos, 'PHOTO')) {
    picture[ph.group] = picture[ph.group] || ph.id;
  }
  const circle = list => {
    const who = [...new Set(list.map(holderOf).filter(Boolean).map(id => first[id]))];
    return who.includes('You') ? ['You', ...who.filter(n => n !== 'You')] : who;
  };
  return rowsOf(data.parties, 'GROUP').map(g => {
    const tickets = members[g.id] || [];
    const availability = availabilityOf(g, tickets.length, now);
    const going = circle(tickets);
    const waiting = circle(members[g.waitlist] || []);
    const mine = going.length ? 'going' : waiting.length ? 'waitlisted' : '';
    return {
      id: g.id, title: g.name, link: g.celebrate_href, start: g.start.slice(0, 10), startAt: g.start, endAt: g.end || g.start,
      image: picture[g.id] ? `/api/blob/${picture[g.id]}/image` : '', imageApp: 'celebrate',
      availability, mine, call: mineWords(mine, mine === 'going' ? going : waiting) || callWords[availability] || '',
    };
  });
}
