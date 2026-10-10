import {el} from '/elements.js';
import {api} from '/api.js';

let listed;

export function policyList() {
  listed ??= api('GET', '/api/policies').then(out => out.clauses);
  return listed;
}

export function clauseQuery(clause) {
  const t = clause.table;
  if (!t || t === '*') {
    return '';
  }
  switch (clause.kind) {
    case 'show':
    case 'read':
    case 'read columns':
      return `(from ${t} @row (where ${clause.condition}))`;
    case 'insert':
      return `(from ${t} @new (where ${clause.condition}))`;
    case 'delete':
      return `(from ${t} @old (where ${clause.condition}))`;
    case 'set':
      return `(from ${t} @old (where (exists ${t} @new (= @new.id @old.id) ${clause.condition})))`;
  }
  return '';
}

export function queryHref(text) {
  return `/query?q=${encodeURIComponent(text)}`;
}

export function clauseHref(index) {
  return `/policies#clause-${index}`;
}

export function actorOf(c) {
  if (c.kind === 'define') {
    return 'definitions';
  }
  if (c.kind === 'show') {
    return 'consent';
  }
  if (c.actor) {
    return c.actor;
  }
  return c.condition === 'false' ? 'nobody' : 'everyone';
}

export function definitions(clauses) {
  const out = new Map();
  clauses.forEach((c, i) => {
    if (c.kind === 'define') {
      out.set(c.name, i);
    }
  });
  return out;
}

const token = /("(?:[^"\\]|\\.)*")|(@[^\s()"]+)|([()])|(-?\d+(?:\.\d+)?(?![^\s()]))|([^\s()"]+)|(\s+)/g;

export function highlight(text, defined = new Map()) {
  const out = el('code', 'sexp');
  let head = false;
  for (const [, string, at, paren, number, name, space] of text.matchAll(token)) {
    if (space) {
      out.append(space);
      continue;
    }
    if (paren) {
      out.append(el('span', 'paren', paren));
      head = paren === '(';
      continue;
    }
    const call = head;
    head = false;
    if (string) {
      out.append(el('span', 'str', string));
    } else if (at) {
      out.append(el('span', 'at', at));
    } else if (number) {
      out.append(el('span', 'num', number));
    } else if (defined.has(name)) {
      const a = el('a', 'def', name);
      a.href = clauseHref(defined.get(name));
      out.append(a);
    } else if (call) {
      out.append(el('span', 'call', name));
    } else if (name === 'true' || name === 'false') {
      out.append(el('span', 'const', name));
    } else if (/^[A-Z][A-Z_]*$/.test(name)) {
      out.append(el('span', 'table', name));
    } else {
      out.append(el('span', 'path', name));
    }
  }
  return out;
}

export function actorLabel(key, defined) {
  if (key === 'everyone') {
    return el('span', 'actor-word', 'everyone');
  }
  if (key === 'nobody') {
    return el('span', 'actor-word', 'nobody');
  }
  return highlight(key, defined);
}
