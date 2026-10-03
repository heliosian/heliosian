import {api} from '/api.js';

let listed;

export function policyList() {
  listed ??= api('GET', '/api/policies').then(out => out.clauses);
  return listed;
}

export function clauseQuery(clause) {
  const t = clause.table;
  switch (clause.kind) {
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
