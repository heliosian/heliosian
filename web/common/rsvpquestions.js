import {queries, rowsOf, write} from '/rail.js';
import {host} from '/guesthost.js';
import {replyKey} from '/questionfields.js';

function questionOf(q) {
  return {
    id: q.id, order: q.order, text: q.text, kind: q.kind, choices: (q.choices || '').split('\n').map(c => c.trim()).filter(Boolean),
    asked: q.asked, required: q.required === 'Yes', goingOnly: q.going_only === 'Yes',
  };
}

export function questionQueries(eventId) {
  const id = JSON.stringify(eventId);
  return {
    questions: `(from QUESTION (where (= group ${id})) (order order asc) (columns text kind choices asked required going_only))`,
    replies: `(from ANSWER (where (in question (select QUESTION.id (= group ${id})))) (columns question person family value))`,
    family: '(from GROUP (where (= kind "family") (in id (select MEMBER.group (= person @viewer)))) (columns name))',
  };
}

export function questionParts(a) {
  const replies = {};
  for (const r of rowsOf(a.replies, 'ANSWER')) {
    replies[replyKey(r.question, r.person, r.family)] = r;
  }
  return {questions: rowsOf(a.questions, 'QUESTION').map(questionOf), replies, family: rowsOf(a.family, 'GROUP')[0] || null, save: saveReply};
}

export async function loadQuestions(eventId, people) {
  const ids = people.map(p => JSON.stringify(p.id)).join(' ');
  const a = await queries({
    ...questionQueries(eventId),
    students: `(from PERSON (where (in id ${ids}) (student id)) (columns name_show))`,
    children: `(from PERSON (where (in id ${ids}) child) (columns name_show))`,
  });
  const kids = new Set([...a.students.result, ...a.children.result]);
  return {...questionParts(a), household: people.map(p => ({...p, kid: kids.has(p.id)}))};
}

function stamp() {
  const d = new Date();
  const pad = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

async function saveReply(question, target, value, existing) {
  if (existing && !value) {
    await write([{delete: existing.id}]);
    return;
  }
  if (existing) {
    await write([{set: existing.id, cells: {value, added: stamp(), added_by: host.me().id}}]);
    return;
  }
  if (value) {
    await write([{insert: 'ANSWER', row: {question: question.id, ...target, value, added: stamp(), added_by: host.me().id}}]);
  }
}
