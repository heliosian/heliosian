import {queries} from '/rail.js';

const adminApps = ['who', 'team', 'when', 'home'];

export async function me() {
  const a = await queries({
    meEmail: '(from PERSON_EMAIL (where (= person @viewer) primary) (columns address))',
    meAdmin: `(from APP @a (where (in key ${adminApps.map(k => JSON.stringify(k)).join(' ')}) (or (super_admin) (exists EFFECTIVE_MEMBER (= group @a.admins) (= person @viewer)))) (columns key))`,
  });
  const email = (Object.values(a.meEmail.resources.PERSON_EMAIL || {})[0] || {}).address || '';
  const keys = a.meAdmin.result.map(id => a.meAdmin.resources.APP[id].key);
  return {email, allowances: keys.map(k => k + '.admins')};
}
