import {query} from '/data.js';

let asked = null;

export function directory() {
  asked = asked || query('/api/people?listed&include=partners,children,parents,siblings').catch(err => {
    asked = null;
    throw err;
  });
  return asked;
}

export async function listed() {
  const dir = await directory();
  return dir.data.map(dir.get);
}

export async function personByEmail(email) {
  return (await listed()).find(p => p.email === email) || null;
}

function firstName(name) {
  return (name || '').trim().split(/\s+/)[0];
}

export function contactLine(dir, p) {
  if (p.isStudent) {
    return [p.grade, p.classroom].filter(Boolean).join(' · ');
  }
  if (p.isParent) {
    const kids = dir.follow(p, 'children').filter(Boolean).map(k => k.grade ? `${firstName(k.fullName)} (${k.grade})` : firstName(k.fullName));
    if (kids.length) {
      return 'Parent to ' + kids.join(', ');
    }
  }
  return p.words || '';
}
