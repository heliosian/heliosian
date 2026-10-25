import {queries, rowsOf} from '/rail.js';

const personColumns = 'name_show name_short photo_updated facts facts_updated';

export function agedPast(updated, years) {
  const when = Date.parse(updated);
  return Number.isNaN(when) || Date.now() - when > years * 365.25 * 24 * 60 * 60 * 1000;
}

function firstName(name) {
  return (name || '').trim().split(/\s+/)[0];
}

export async function viewerAlerts() {
  const a = await queries({
    staleMe: `(from PERSON (where (= id @viewer)) (columns ${personColumns} address_consent phone_consent vc_address_visibility vc_phone_visibility))`,
    staleHousehold: `(from PERSON (where (in id (household @viewer))) (columns ${personColumns}))`,
    staleRoles: '(from EFFECTIVE_MEMBER (where (in group (slug "students" "parents" "staff")) (or (= person @viewer) (in person (household @viewer)))) (include group) (columns group person))',
    staleFamily: '(from MEMBER (where (= person @viewer) (= member "yes") (= group.kind "family")) (columns group))',
    stalePhotos: '(from PHOTO (where ready (or (= person @viewer) (in person (household @viewer)) (in group (select MEMBER.group (= person @viewer) (= group.kind "family"))))) (columns person group))',
    staleYears: '(from SETTING (where (= app "platform") (in key "Photo Stale Years" "Facts Stale Years")) (columns key value))',
  });
  const me = rowsOf(a.staleMe, 'PERSON')[0];
  if (!me) {
    return {stale: [], privacy: []};
  }
  const groups = a.staleRoles.resources.GROUP || {};
  const roles = {students: new Set(), parents: new Set(), staff: new Set()};
  for (const m of rowsOf(a.staleRoles, 'EFFECTIVE_MEMBER')) {
    roles[(groups[m.group] || {}).slug].add(m.person);
  }
  const years = Object.fromEntries(rowsOf(a.staleYears, 'SETTING').map(s => [s.key, Number(s.value)]));
  const photographed = new Set(rowsOf(a.stalePhotos, 'PHOTO').map(p => p.person || p.group));
  const family = (rowsOf(a.staleFamily, 'MEMBER')[0] || {}).group || '';
  const stale = [];
  if (!roles.staff.has(me.id) || roles.parents.has(me.id)) {
    for (const p of [me, ...rowsOf(a.staleHousehold, 'PERSON')]) {
      if (!roles.students.has(p.id)) {
        continue;
      }
      const whose = p.id === me.id ? 'Your' : `${firstName(p.name_short || p.name_show)}'s`;
      if (!photographed.has(p.id) || agedPast(p.photo_updated, years['Photo Stale Years'])) {
        stale.push(`${whose} photo`);
      }
      if (!p.facts || agedPast(p.facts_updated, years['Facts Stale Years'])) {
        stale.push(`${whose} facts`);
      }
    }
    if (family && !photographed.has(family)) {
      stale.push('Your family photo');
    }
  }
  const privacy = [];
  if (family) {
    if (me.address_consent !== 'shared' && me.vc_address_visibility && me.vc_address_visibility !== 'hidden') {
      privacy.push('address');
    }
    if (me.phone_consent !== 'shared' && me.vc_phone_visibility && me.vc_phone_visibility !== 'hidden') {
      privacy.push('phone number');
    }
  }
  return {stale, privacy};
}
