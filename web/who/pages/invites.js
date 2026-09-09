import {state, byEmail} from '../state.js';
import {el, svg, firstName, lastName, hue, slugify, csvField, copyGlyph} from '../dom.js';
import {familyOf, familyLink, familySearchText} from '../families.js';
import {personLink} from '../people.js';
import {tagNames} from '../tags.js';
import {saveTagRelations} from '../storage.js';
import {anyFiltersActive, matchesFilters, familyMatchesFilters, roleChips, facetDropdown, gradeOptions, tagRelationOptions} from '../filters.js';
import {resetMain} from '../chrome.js';

// Combines a list of people into one greeting phrase. Two or more people who all
// share a last name are introduced by first name only, with the surname stated once
// at the end ("Alice & Bob McDowell", "Alice, Bob & Carol McDowell"); anyone who
// doesn't share that surname (blended families, a spouse who kept their own name) is
// spelled out in full instead, so nobody's identity is silently merged into someone
// else's. A single person is just their full name.
function joinFamilyNames(people) {
  if (!people.length) {
    return '';
  }
  if (people.length === 1) {
    return people[0].fullName;
  }
  const surname = lastName(people[0].fullName);
  const shared = surname && people.every(p => lastName(p.fullName) === surname);
  const names = people.map(p => shared ? firstName(p.fullName) : p.fullName);
  const line = names.length === 2 ? names.join(' & ') : `${names.slice(0, -1).join(', ')} & ${names[names.length - 1]}`;
  return shared ? `${line} ${surname}` : line;
}

// Like joinFamilyNames, but first names only, with no trailing surname even
// when everyone shares one - what "First name" (as opposed to "Full name")
// means once Include siblings turns a single kid's individual greeting into
// a group of two or more (see buildGreeting).
function joinFirstNames(people) {
  if (!people.length) {
    return '';
  }
  const names = people.map(p => firstName(p.fullName));
  return names.length === 1 ? names[0] : `${names.slice(0, -1).join(', ')} & ${names[names.length - 1]}`;
}

// The _Greetings sheet's Format column is a worked example, not a {{ token }}
// template: it's written against one made-up family - parents Pat and Quinn,
// kids Ali and Bo, everyone surnamed Ender - so writing or editing a greeting
// means describing what it would look like for that one family rather than
// learning template syntax. These are the exact phrases recognized in a
// Format string and what each is replaced with - read from the sheet's own
// "Whole family" / "Kids only" / "Adults only" / "Full name" / "First name"
// rows (via refreshGreetingPhrases, called once loadInviteSystems has the
// greetings) rather than hardcoded here a second time, so the reference
// family lives in exactly one place and editing those rows is all it takes
// to change it. The literals are only a fallback for a sheet missing one of
// those rows - matched longest-first so a bare "Pat" inside "Pat & Quinn
// Ender" doesn't get consumed by the shorter phrase before the longer one
// gets a chance to match.
let GREETING_WHOLE_FAMILY = 'Ali, Bo, Pat & Quinn Ender';
let GREETING_KIDS = 'Ali & Bo Ender';
let GREETING_ADULTS = 'Pat & Quinn Ender';
let GREETING_FULL_NAME = 'Pat Ender';
let GREETING_FIRST_NAME = 'Pat';

function refreshGreetingPhrases() {
  const phraseOf = (name, fallback) => {
    const g = inviteGreetings.find(g => g.name === name);
    return g && g.format ? g.format : fallback;
  };
  GREETING_WHOLE_FAMILY = phraseOf('Whole family', GREETING_WHOLE_FAMILY);
  GREETING_KIDS = phraseOf('Kids only', GREETING_KIDS);
  GREETING_ADULTS = phraseOf('Adults only', GREETING_ADULTS);
  GREETING_FULL_NAME = phraseOf('Full name', GREETING_FULL_NAME);
  GREETING_FIRST_NAME = phraseOf('First name', GREETING_FIRST_NAME);
}

// The standalone Ender token (see substituteNameTokens) always stands for
// the family's own last name, not just one shared by everyone in the
// household - unlike joinFamilyNames' surname (which only appends one when
// the whole group agrees, falling back to full names otherwise), a bare
// "Ender" has nowhere else to fall back to, so it takes whichever surname
// the greeting would naturally be filed under: the primary adult's for a
// family (kids listed first in the group only for name-joining purposes,
// not surname), or the addressed person's own for an individual.
function greetingSurname({kids, adults, person}) {
  if (person) {
    return lastName(person.fullName);
  }
  const primary = adults[0] || kids[0];
  return primary ? lastName(primary.fullName) : '';
}

// splitLastFirst divides a list of people's first names into "everyone but
// the last, comma-joined" and "the last one" - what Ali/Pat and Bo/Quinn
// respectively stand for once they're used loosely rather than as part of
// one of the fixed phrases above. Lines up with joinFamilyNames' own
// comma-then-last shape, so a Format that keeps that same punctuation around
// the tokens reads identically to using the whole-list phrase would.
function splitLastFirst(people) {
  if (!people.length) {
    return {rest: '', last: ''};
  }
  const names = people.map(p => firstName(p.fullName));
  return {rest: names.slice(0, -1).join(', '), last: names[names.length - 1]};
}

// Beyond the fixed phrases buildGreeting checks first (which cover the
// common, cleanly-grouped cases), each individual example name is also
// recognized on its own, wherever it lands in the text - so a Format that
// wraps or rearranges them in its own words (say, "The Ender Family (Ali,
// Bo, Pat & Quinn)") still personalizes per family, instead of only working
// when it reproduces one of the exact phrases verbatim. Runs after those
// fixed-phrase checks, on whatever text they left untouched, so a template
// that does use one of the exact phrases keeps that phrase's own clean
// grouping (correct for 1, 2, or more people) rather than this word-by-word
// fallback's cruder splitting.
// Ali and Pat each stand for "everyone but the last one" in their group,
// which is empty for the common case of a single kid or single parent - a
// plain substitution would then leave whatever separator conventionally
// follows the token (", " in a list, " & " right before the last name)
// stranded at the start (e.g. "(Ali, Bo" -> "(, Bo"). When there's nothing
// to say, this swallows that separator along with the token instead, so it
// reads "(Bo" the way someone writing it by hand would have.
function replaceRestToken(text, token, value) {
  if (value) {
    return text.replace(new RegExp(`\\b${token}\\b`, 'g'), value);
  }
  return text
    .replace(new RegExp(`\\b${token}\\b,\\s*`, 'g'), '')
    .replace(new RegExp(`\\b${token}\\b\\s*&\\s*`, 'g'), '')
    .replace(new RegExp(`\\b${token}\\b`, 'g'), '');
}

function substituteNameTokens(text, {kids, adults, person}) {
  if (!person) {
    const kidsSplit = splitLastFirst(kids);
    const adultsSplit = splitLastFirst(adults);
    text = replaceRestToken(text, 'Ali', kidsSplit.rest);
    text = text.replace(/\bBo\b/g, kidsSplit.last);
    text = replaceRestToken(text, 'Pat', adultsSplit.rest);
    text = text.replace(/\bQuinn\b/g, adultsSplit.last);
  }
  // Also matches "Enders" (the plural a template might use for "the Enders")
  // - only the root swaps for the real surname, so the trailing s (if any)
  // stays put rather than needing its own token.
  const surname = greetingSurname({kids, adults, person});
  return text.replace(/\bEnder(s?)\b/g, `${surname}$1`);
}

// Turns one greeting's Format into real text for a family (kids/adults) or a
// single person (optionally with their siblings folded in too - see
// personInviteParams and Include siblings), by substituting whichever example
// phrases or names actually appear in it - a Format that doesn't use any of
// them at all (a fixed greeting with no names) just passes through
// unchanged. The joined-kids phrase falls back to the adults when there are
// no kids, so a childless couple's "Family of the kids"-style greeting
// doesn't come out blank.
function buildGreeting(format, {kids = [], adults = [], person, siblings = []} = {}) {
  let text = format;
  if (GREETING_WHOLE_FAMILY && text.includes(GREETING_WHOLE_FAMILY)) {
    text = text.replaceAll(GREETING_WHOLE_FAMILY, joinFamilyNames([...kids, ...adults]));
  }
  if (GREETING_KIDS && text.includes(GREETING_KIDS)) {
    text = text.replaceAll(GREETING_KIDS, joinFamilyNames(kids.length ? kids : adults));
  }
  if (GREETING_ADULTS && text.includes(GREETING_ADULTS)) {
    text = text.replaceAll(GREETING_ADULTS, joinFamilyNames(adults));
  }
  if (person) {
    // With no siblings (the common case - an adult's row, or Include siblings
    // off), this is just person's own name either way, same as always.
    const group = [person, ...siblings];
    if (GREETING_FULL_NAME && text.includes(GREETING_FULL_NAME)) {
      text = text.replaceAll(GREETING_FULL_NAME, joinFamilyNames(group));
    }
    if (GREETING_FIRST_NAME && text.includes(GREETING_FIRST_NAME)) {
      text = text.replaceAll(GREETING_FIRST_NAME, joinFirstNames(group));
    }
  }
  return substituteNameTokens(text, {kids, adults, person, siblings});
}

// Individual mode always uses greetings checked Individual - there's no
// family left to build a joint name from once every person gets their own
// row. Group mode uses ones checked Grouped, unless the system itself has no
// way to address a group at all (Punchbowl, Partiful - see
// system.supportsGroups), in which case even a family's one row can only be
// addressed with its primary contact's own name. Grouped and Individual are
// independent (see GreetingTemplate in invites.go) - a greeting can be, and a
// fixed line with no names in it typically would be, both at once.
function greetingFormatsFor(inviteBy, supportsGroups) {
  const wantGrouped = inviteBy === 'group' && supportsGroups;
  return inviteGreetings.filter(g => wantGrouped ? g.grouped : g.individual);
}

// Shared by both invite-by modes: builds the token params a template can pull
// from - primary_email/first/last/phone from whoever is standing in as this
// row's addressee (the family's primary contact in Group mode, or the person
// themself in Individual mode), plus member_1..N, every other person
// flattened into numbered slots ("Name <email>" or just "Name" - what
// Greenvelope/Evite/Punchbowl/Partiful's one-row-per-addressee templates use).
// `people` (addressee first, then members) is the alternate shape a template
// that repeats one row per person (Paperless Post) uses instead - see
// applyInviteTemplate, which decides which shape a given template wants.
function buildInviteParams(addressee, addresseeContact, greeting, members) {
  const params = {
    greeting,
    primary_email: addresseeContact,
    primary_first_name: firstName(addressee.fullName),
    primary_last_name: lastName(addressee.fullName),
    primary_phone: addressee.phoneMasked ? '' : (addressee.phone || ''),
  };
  members.forEach((m, i) => {
    params[`member_${i + 1}`] = m.contact ? `${m.name} <${m.contact}>` : m.name;
  });
  return {
    greeting,
    people: [{name: addressee.fullName, contact: addresseeContact}, ...members],
    params,
  };
}

// A kid's contact, per state.gvKidEmail: off lists them without a way to
// reach them directly, on gives them their own reachable address (blank if
// they don't have one, or it's a masked Veracross placeholder).
function kidContact(kid) {
  return state.gvKidEmail && kid.email && !kid.emailMasked ? kid.email : '';
}

// A family's adults with a real, reachable email - a Veracross placeholder
// address doesn't belong on a mailing list, matching emailEntries - deduped,
// since a family's own adultEmails list is assumed unique but a caller
// merging more than one source (none currently do) shouldn't double-invite
// someone as a result.
function parentContactsOf(adultEmails) {
  const emails = [...new Set(adultEmails || [])];
  return emails.map(e => byEmail[e]).filter(a => a && !a.emailMasked);
}

// A family's kids to actually put on the invite. With Include siblings on,
// that's everyone. Off, it's still not necessarily nobody: if the current
// filters/search single out specific students (a grade, a tag, a name typed
// into search), whichever of this family's kids match are the ones actually
// being invited, not incidental siblings - they stay on regardless, and it's
// only their non-matching siblings that Include siblings is left to add back
// in. With no filter or search narrowing things down at all, there's no
// "invited kid" to distinguish from a sibling, so off just means no kids.
function invitedKids(family) {
  const kids = (family.kidEmails || []).map(e => byEmail[e]).filter(Boolean);
  if (state.gvSiblings) {
    return kids;
  }
  if (!anyFiltersActive() && !state.q) {
    return [];
  }
  return kids.filter(k => matchesFilters(k) &&
    (!state.q || k.fullName.toLowerCase().includes(state.q) || k.email.toLowerCase().includes(state.q)));
}

// Group mode: one row per family. The primary contact is the first listed
// adult with a real, reachable email (matching emailEntries' rule that a
// Veracross placeholder address doesn't belong on a mailing list); everyone
// else (other adults, then kids per invitedKids) becomes a member. Returns
// null for a family with no usable primary contact at all.
function familyInviteParams(family) {
  const adults = (family.adultEmails || []).map(e => byEmail[e]).filter(p => p && !p.emailMasked);
  if (!adults.length) {
    return null;
  }
  const kids = invitedKids(family);
  const [primary, ...otherAdults] = adults;
  const format = inviteGreetings.find(g => g.name === state.gvGreeting) || greetingFormatsFor('group', true)[0];
  const greeting = format ? buildGreeting(format.format, {kids, adults}) : '';
  const members = otherAdults.map(a => ({name: a.fullName, contact: a.email}))
    .concat(kids.map(k => ({name: k.fullName, contact: kidContact(k)})));
  const anchor = kids[0] || primary;
  return {
    ...buildInviteParams(primary, primary.email, greeting, members),
    linkHref: familyLink(family.key),
    sortKey: lastName(anchor.fullName) + ' ' + firstName(anchor.fullName),
  };
}

// One real contact email can only ever appear once in Individual mode's
// output - never two rows to the same inbox, whatever caused the collision
// (two siblings who each separately qualify, a parent who also independently
// qualifies as their own invitee, or - with Include siblings off - two
// separately-matching kids who just happen to share a parent). Rather than
// dropping whichever candidate loses the collision (silently losing a real
// kid's invite), every candidate reaching the same contact is merged into one
// row that names everyone routed there - see mergeCandidates.
//
// A candidate is {contact, person}: person is who this candidate is "about"
// (whose name should count toward the merged greeting), contact is where it
// would be delivered - blank if a student has no parent on record to route
// through (never merged with anything else, so that student's own row still
// shows up rather than silently vanishing).
function individualCandidates(p) {
  if (!p.isStudent) {
    return p.emailMasked ? [] : [{contact: p.email, person: p}];
  }
  const family = familyOf(p);
  // With Include siblings on, every kid in the household is folded into the
  // same set of candidates the first time any of them is reached - the later
  // merge pass then naturally collapses them into one row per parent, same as
  // it would for any other collision.
  let kids = [p];
  if (state.gvSiblings && family) {
    kids = invitedKids(family);
  }
  const parents = family ? parentContactsOf(family.adultEmails) : [];
  if (!parents.length) {
    return kids.map(kid => ({contact: '', person: kid}));
  }
  const candidates = [];
  for (const parent of parents) {
    for (const kid of kids) {
      candidates.push({contact: parent.email, person: kid});
    }
  }
  return candidates;
}

// Turns a merged group of people (everyone whose invite collided on the same
// contact - see individualCandidates) into one row: the first person is the
// addressee (whoever's own name/email columns represent the row - an
// independently-matching adult, if there is one, since candidates are built
// adults-first), everyone else is folded into the greeting as "siblings" (see
// buildGreeting - the name is a holdover from the common case, but this works
// for any group of people sharing one contact, related or not).
function buildMergedEntry(people, contact) {
  const addressee = people[0];
  const format = inviteGreetings.find(g => g.name === state.gvGreeting) || greetingFormatsFor('individual', false)[0];
  const greeting = format ? buildGreeting(format.format, {person: addressee, siblings: people.slice(1)}) : '';
  return {
    ...buildInviteParams(addressee, contact, greeting, []),
    linkHref: personLink(addressee),
    sortKey: lastName(addressee.fullName) + ' ' + firstName(addressee.fullName),
  };
}

// Groups every candidate (see individualCandidates) by contact email and
// builds one merged row per group (see buildMergedEntry) - the one place
// Individual mode's "never the same email twice" rule is actually enforced.
// A blank contact (no parent on record) is never merged with anything else,
// each becomes its own row.
function mergeCandidates(candidates) {
  const byContact = new Map();
  const rows = [];
  for (const c of candidates) {
    if (!c.contact) {
      rows.push(buildMergedEntry([c.person], ''));
      continue;
    }
    if (!byContact.has(c.contact)) {
      byContact.set(c.contact, []);
    }
    const people = byContact.get(c.contact);
    if (!people.some(x => x.email === c.person.email)) {
      people.push(c.person);
    }
  }
  for (const [contact, people] of byContact) {
    rows.push(buildMergedEntry(people, contact));
  }
  return rows;
}

// Every family (Group mode) or person (Individual mode) with a usable
// addressee, filtered and searched the same way the rest of the list pages
// are - familyMatchesFilters/familySearchText for Group mode (the same ones
// the Families tab and the map rely on), plain matchesFilters/name-or-email
// search for Individual mode, matching emailEntries. Sorted by last name -
// the anchor kid's, for a family, matching the order the original Greenvelope
// sheet this page grew out of already used.
function invitesEntries() {
  const rows = [];
  if (state.gvInviteBy === 'individual') {
    const matches = p => matchesFilters(p) && (p.fullName.toLowerCase().includes(state.q) || p.email.toLowerCase().includes(state.q));
    const candidates = [];
    // Adults first, deliberately: when an adult's own invite and their kid's
    // routed-through-them invite land on the same contact and get merged, the
    // adult - added to the candidate list first - is the one buildMergedEntry
    // picks as the addressee.
    for (const p of state.model.people) {
      if (!p.isStudent && matches(p)) {
        candidates.push(...individualCandidates(p));
      }
    }
    // With Include siblings on, a household's candidates all get generated
    // together the first time any of its kids is reached (individualCandidates
    // expands to the whole household); mergedFamilies then skips that
    // household's other kids so they don't generate the same candidates again.
    const mergedFamilies = new Set();
    for (const p of state.model.people) {
      if (!p.isStudent || !matches(p)) {
        continue;
      }
      if (state.gvSiblings) {
        const family = familyOf(p);
        if (family) {
          if (mergedFamilies.has(family.key)) {
            continue;
          }
          mergedFamilies.add(family.key);
        }
      }
      candidates.push(...individualCandidates(p));
    }
    rows.push(...mergeCandidates(candidates));
  } else {
    for (const family of Object.values(state.model.families)) {
      if (!familyMatchesFilters(family.key) || !familySearchText(family).includes(state.q)) {
        continue;
      }
      const entry = familyInviteParams(family);
      if (entry) {
        rows.push(entry);
      }
    }
  }
  rows.sort((a, b) => a.sortKey.localeCompare(b.sortKey));
  return rows;
}

// Fills a template string's {{ token }} placeholders from params; a token with
// no value (including one this app doesn't compute at all, like Paperless
// Post's optional Message column) just renders blank rather than erroring.
function fillTemplate(template, params) {
  return (template || '').replace(/\{\{\s*([\w.]+)\s*\}\}/g, (_, key) => params[key] ?? '');
}

// Whether a template's row(s) use the per-member tokens (member_name/
// member_contact) rather than the numbered member_N slots - a template that
// does (Paperless Post's, so far) wants one output row per family MEMBER, not
// one row per family; see that system's Notes in the Invite List Builder sheet.
function templateRepeatsPerMember(rows) {
  return rows.some(row => row.some(cell => /\{\{\s*member_(name|contact)\s*\}\}/.test(cell || '')));
}

// Whether a template actually uses {{ greeting }} anywhere - the Couple /
// Family greeting picker only matters, and only shows, for a system whose
// template references it. A system that greets some other way (or not at all)
// has no use for the setting.
function templateUsesGreeting(rows) {
  return rows.some(row => row.some(cell => /\{\{\s*greeting\s*\}\}/.test(cell || '')));
}

// Turns one invite system's template (its tab's header + template row, as read
// from the Invite List Builder sheet by the server) into real output rows for
// the given family entries - the one place a sheet template plus computed
// family params actually becomes a CSV. Row shape (one per family, or one per
// family member) is inferred from which tokens the template uses, not
// hardcoded per system, so a new system's tab just needs the right tokens.
function applyInviteTemplate(system, entries) {
  const templateRow = system.rows[0] || [];
  const perMember = templateRepeatsPerMember(system.rows);
  const rows = [];
  for (const entry of entries) {
    if (!perMember) {
      rows.push({entry, cells: system.header.map((_, i) => fillTemplate(templateRow[i], entry.params))});
      continue;
    }
    for (const person of entry.people) {
      const params = {...entry.params, member_name: person.name, member_contact: person.contact};
      rows.push({entry, cells: system.header.map((_, i) => fillTemplate(templateRow[i], params))});
    }
  }
  return rows;
}

// Cached after the first successful fetch - the templates only change when
// someone edits the Invite List Builder sheet, not per page visit.
let inviteSystems = null;
let inviteGreetings = [];
let inviteLoadError = '';

async function loadInviteSystems() {
  if (inviteSystems) {
    return inviteSystems;
  }
  const res = await fetch('/api/directory/invite-templates');
  const body = await res.json().catch(() => ({}));
  inviteSystems = body.systems && body.systems.length ? body.systems : [];
  inviteGreetings = body.greetings || [];
  inviteLoadError = body.error || '';
  if (body.error) {
    console.error('invite templates:', body.error);
  }
  refreshGreetingPhrases();
  return inviteSystems;
}

// The formatted-CSV export page: pick a destination system (Greenvelope, Evite,
// ... - whatever the Invite List Builder sheet's _Services tab lists), then export
// the currently filtered families through that system's own template. One row
// per family, or one row per family member, depending on the template - see
// applyInviteTemplate.
export function renderGreenvelopePage() {
  const main = resetMain();

  const pageHeader = el('div', 'page-header container page-header-list');
  const titleWrap = el('div');
  titleWrap.append(el('h1', 'page-title', 'Invites'));
  titleWrap.append(el('div', 'page-subtitle', 'Export a formatted CSV of families, grouped and greeted the way you choose, for invitations.'));
  pageHeader.append(titleWrap);
  main.append(pageHeader);

  const settings = el('div', 'gv-settings container');
  const loading = el('div', 'gv-setting-group', 'Loading invite templates…');
  settings.append(loading);
  main.append(settings);

  const content = el('div', 'content container');
  main.append(content);

  loadInviteSystems().then(systems => {
    if (!systems.length) {
      settings.replaceChildren();
      content.replaceChildren();
      settings.append(el('div', 'gv-setting-group', inviteLoadError
        ? `Couldn't load invite templates: ${inviteLoadError}`
        : 'No invite templates are set up yet - add one to the Invite List Builder sheet\'s _Services tab.'));
      return;
    }
    if (!systems.some(s => s.name === state.gvSystem)) {
      state.gvSystem = systems[0].name;
    }

    // Rebuilds the whole page: needed whenever which system is selected
    // changes, since that decides which other settings (right now, just the
    // Couple / Family greeting picker) even apply. Everything else (role
    // chips, search, Grade/Classroom/Tags) only needs the cheaper renderGrid,
    // wired up below - renderGrid is a hoisted declaration, so it's already
    // callable here even though it's defined later in this same function.
    function renderAll() {
      settings.replaceChildren();
      content.replaceChildren();
      renderInviteSettings(systems, settings, renderAll, renderGrid);

      const header = el('div', 'content-header content-header-solo');
      const controls = el('div', 'controls');
      controls.append(roleChips(renderGrid));
      const search = el('div', 'search');
      search.append(svg('search'));
      const input = el('input');
      input.placeholder = 'Search';
      input.value = state.q;
      input.addEventListener('input', () => {
        state.q = input.value.trim().toLowerCase();
        renderGrid();
      });
      search.append(input);
      controls.append(
        facetDropdown('Grade', gradeOptions(), state.filterGrades, renderGrid),
        facetDropdown('Classroom', state.model.classrooms.map(c => c.name), state.filterClassrooms, renderGrid),
      );
      if (tagNames().length) {
        // Rebuilds the whole page, not just the grid - selecting down to (or
        // away from) exactly one tag changes whether the Include control just
        // below even applies.
        controls.append(facetDropdown('Tags', tagNames(), state.filterTags, renderAll));
      }
      // Who to include beyond the tagged person themselves - only meaningful
      // for a single tag; with several selected at once (or none) there's no
      // one list to pull relatives in from. Mirrors renderListPage's identical
      // control exactly, down to persisting the choice per tag.
      if (state.filterTags.size === 1) {
        const [activeTag] = state.filterTags;
        controls.append(facetDropdown('Include', tagRelationOptions, state.filterTagRelations, () => {
          saveTagRelations(activeTag, state.filterTagRelations);
          renderGrid();
        }));
      }
      const download = el('a', 'filter-button email-download');
      download.title = 'Download what the list currently shows';
      download.append(svg('download'), el('span', '', 'CSV'));
      controls.append(search, download);
      header.append(controls);
      content.append(header);

      const grid = el('div');
      content.append(grid);

      function renderGrid() {
        grid.replaceChildren();
        grid.className = '';
        const system = systems.find(s => s.name === state.gvSystem) || systems[0];
        const entries = invitesEntries();
        const rows = applyInviteTemplate(system, entries);

        const csvLines = rows.map(r => r.cells.map(csvField).join(','));
        if (system.headerRow) {
          csvLines.unshift(system.header.map(csvField).join(','));
        }
        download.href = 'data:text/csv;charset=utf-8,' + encodeURIComponent(csvLines.join('\n'));
        download.download = slugify(system.name) + '.csv';

        if (!rows.length) {
          grid.append(el('div', 'empty', 'No matches.'));
          return;
        }

        grid.className = 'email-holder';
        const table = el('table', 'email-table');
        const thead = el('thead');
        const headRow = el('tr');
        // Whichever column the template maps to {{ greeting }} gets the
        // format picker right in its header, instead of a plain label - the
        // same choice as the Couple / Family greeting setting above (changing
        // either updates both, since both just read/write state.gvGreeting).
        const greetingCol = system.rows[0] ? system.rows[0].findIndex(cell => /\{\{\s*greeting\s*\}\}/.test(cell || '')) : -1;
        const leadTh = el('th', 'email-copy-cell');
        const copyTable = el('button', 'email-copy-columns');
        copyTable.title = 'Copy the whole table to the clipboard';
        copyTable.append(svg('copy'));
        copyTable.addEventListener('click', () => {
          const lines = [system.header.join('\t')].concat(rows.map(r => r.cells.join('\t')));
          navigator.clipboard.writeText(lines.join('\n'));
          copyTable.classList.add('copied');
          copyTable.replaceChildren(svg('check'));
          setTimeout(() => {
            copyTable.classList.remove('copied');
            copyTable.replaceChildren(svg('copy'));
          }, 1200);
        });
        leadTh.append(copyTable);
        headRow.append(leadTh);
        system.header.forEach((label, i) => {
          const th = el('th');
          if (i === greetingCol) {
            const headSelect = el('select', 'gv-select gv-th-select');
            const formats = greetingFormatsFor(state.gvInviteBy, system.supportsGroups);
            const meEmail = document.body.dataset.userEmail;
            const mine = formats.filter(f => f.createdBy && f.createdBy === meEmail);
            const appendOptions = (parent, list) => {
              for (const f of list) {
                const option = el('option', '', f.name);
                option.value = f.name;
                parent.append(option);
              }
            };
            // Only splits into "Yours"/"Everyone's" once there's actually
            // something of the viewer's own to set apart - otherwise it's
            // just the one flat list it's always been.
            if (mine.length) {
              const mineGroup = el('optgroup');
              mineGroup.label = 'Yours';
              appendOptions(mineGroup, mine);
              headSelect.append(mineGroup);
              const everyoneGroup = el('optgroup');
              everyoneGroup.label = 'Everyone’s';
              appendOptions(everyoneGroup, formats.filter(f => !mine.includes(f)));
              headSelect.append(everyoneGroup);
            } else {
              appendOptions(headSelect, formats);
            }
            const divider = el('option', '', '──────────');
            divider.disabled = true;
            headSelect.append(divider);
            const editOption = el('option', '', '(Edit Greetings)');
            editOption.value = '__new__';
            headSelect.append(editOption);
            headSelect.value = state.gvGreeting;
            headSelect.addEventListener('change', () => {
              if (headSelect.value === '__new__') {
                headSelect.value = state.gvGreeting;
                openGreetingDialog(renderGreenvelopePage);
                return;
              }
              state.gvGreeting = headSelect.value;
              renderAll();
            });
            th.append(headSelect);
          } else {
            th.append(el('span', '', label));
          }
          headRow.append(th);
        });
        thead.append(headRow);
        table.append(thead);
        const tbody = el('tbody');
        rows.forEach((r, rowIndex) => {
          const tr = el('tr');
          const num = el('td', 'email-num');
          num.append(el('span', '', String(rowIndex + 1)), copyGlyph(r.cells.join('\t')));
          tr.append(num);
          r.cells.forEach((value, i) => {
            const td = el('td', i === 0 ? 'email-name' : '', i === 0 ? '' : value);
            if (i === 0) {
              const link = el('a', '', value);
              link.href = r.entry.linkHref;
              td.append(link);
            }
            tr.append(td);
          });
          tbody.append(tr);
        });
        table.append(tbody);
        grid.append(table);
      }
      renderGrid();
      input.focus();
    }
    renderAll();
  });
}

// Real per-service icons (small favicons, not full marketing logos) so the
// destination is recognizable at a glance in the Export for picker. Keyed by
// the exact Display Name from the Invite List Builder's _Services tab; a
// system added there without a matching icon here just falls back to a
// colored initial, the same way a person with no photo does elsewhere in
// this app (see photoOrInitials).
const serviceLogos = {
  Greenvelope: '/services/greenvelope.png',
  Evite: '/services/evite.png',
  'Paperless Post': '/services/paperless-post.png',
  Punchbowl: '/services/punchbowl.png',
  Partiful: '/services/partiful.jpg',
};

function serviceIcon(name) {
  const url = serviceLogos[name];
  if (url) {
    const img = el('img', 'gv-service-logo');
    img.src = url;
    img.alt = '';
    return img;
  }
  const div = el('div', 'gv-service-logo gv-service-fallback', (name[0] || '?').toUpperCase());
  div.style.background = `hsl(${hue(name)} 45% 55%)`;
  return div;
}

// A custom Export for picker in place of a plain <select> - only so each
// system's icon (serviceIcon) can show both on the closed button and in the
// open list, which a native select can't render at all. Built from the app's
// existing .filter-button/.filter-panel/.filter-option classes so the one
// global click-outside handler that already closes every other dropdown in
// the app (see the document click listener in chrome.js) closes this one
// too, for free.
function gvServiceSelect(seg, systems, onChange) {
  const wrap = el('div', 'filter-wrap gv-service-wrap');
  const button = el('button', 'filter-button gv-service-button');
  button.type = 'button';
  const current = systems.find(s => s.name === state.gvSystem) || systems[0];
  button.append(serviceIcon(current.name), el('span', '', current.name), svg('chevron'));
  const panel = el('div', 'filter-panel gv-service-panel');
  panel.hidden = true;
  button.addEventListener('click', () => {
    panel.hidden = !panel.hidden;
    button.classList.toggle('open', !panel.hidden);
  });
  for (const s of systems) {
    const option = el('div', 'filter-option gv-service-option');
    option.append(serviceIcon(s.name), el('span', '', s.name));
    option.addEventListener('click', () => onChange(s.name));
    panel.append(option);
  }
  wrap.append(button, panel);
  seg.append(wrap);
}

// The default starting point for a brand new greeting - a worked example
// that already reads as a real greeting rather than an empty box, using the
// same made-up family (kids Ali & Bo, parents Pat & Quinn, surname Ender) the
// recognized phrases below are drawn from, so it edits cleanly into a real
// one just by trimming or rearranging. Mode-dependent because the phrases
// that actually substitute anything differ: a family's kids/adults are only
// ever populated for a Grouped call, a lone person only for an Individual
// one (see buildGreeting) - defaulting to the wrong kind would silently
// render as flat, unpersonalized text for every row, same as the bug this
// default (and the live preview below) exists to avoid.
function defaultGreetingFormat() {
  return state.gvInviteBy === 'group' ? `The ${GREETING_WHOLE_FAMILY} Family` : `Dear ${GREETING_FIRST_NAME}`;
}

// A second, differently-named made-up family (not Ender) to run a candidate
// Format through live as the person types - proves whether it actually
// personalizes per family rather than just echoing the Ender example back
// unchanged, which would look "correct" without proving the substitution
// fired at all.
const GREETING_PREVIEW_FAMILY = {
  kids: [{fullName: 'Nora Rivera'}, {fullName: 'Theo Rivera'}],
  adults: [{fullName: 'Sam Rivera'}, {fullName: 'Jamie Rivera'}],
};

// The pop-up behind "(Edit Greetings)" at the bottom of the greeting picker
// in the table header - lists whatever custom greetings the viewer has
// already created, lets picking one load it below for editing, and otherwise
// starts a new one. There's no separate name to write: a greeting's Format
// text doubles as its Name (see GreetingTemplate in invites.go), so the only
// field here is the greeting itself. A true modal (built like openCropTool's
// overlay - appended to document.body, closes on backdrop click or Escape)
// rather than an anchored dropdown panel, since the form is too tall to hang
// cleanly off a table header cell. onSaved is called after a successful
// save, with inviteSystems already cleared so the next fetch picks up the
// change.
function openGreetingDialog(onSaved) {
  const overlay = el('div', 'greeting-dialog-overlay');
  const panel = el('div', 'greeting-dialog-panel');

  const close = () => {
    overlay.remove();
    document.removeEventListener('keydown', onKey);
  };
  function onKey(e) {
    if (e.key === 'Escape') {
      close();
    }
  }
  overlay.addEventListener('click', e => {
    if (e.target === overlay) {
      close();
    }
  });
  document.addEventListener('keydown', onKey);

  const header = el('div', 'greeting-dialog-header');
  const headerIcon = el('div', 'greeting-dialog-icon');
  headerIcon.append(svg('sparkles'));
  header.append(headerIcon);
  const headerText = el('div', 'greeting-dialog-header-text');
  headerText.append(
    el('div', 'greeting-dialog-title', 'Edit Greeting'),
    el('div', 'greeting-dialog-subtitle', 'Create a sample greeting for this family.'),
  );
  header.append(headerText);
  const headerClose = el('button', 'greeting-dialog-close');
  headerClose.type = 'button';
  headerClose.setAttribute('aria-label', 'Close');
  headerClose.append(svg('x'));
  headerClose.addEventListener('click', close);
  header.append(headerClose);
  panel.append(header);

  const form = el('form', 'gv-new-greeting-form');

  // Fixed for the life of this dialog - which kind of call (a family's
  // kids/adults, or one person) this greeting will actually be run through
  // is decided by which mode it's saved under, not by anything typed here.
  const grouped = state.gvInviteBy === 'group';

  // original names the existing _Greetings row currently loaded into the
  // field below (so Save edits it in place), or '' while writing a new one.
  let original = '';
  let cards = [];

  const formatLabel = el('label', 'gv-new-greeting-field');
  formatLabel.append(el('span', '', 'Greeting'));
  const formatInput = el('input');
  formatInput.maxLength = 200;
  formatInput.required = true;
  formatLabel.append(formatInput);

  // Runs the candidate text through buildGreeting against a sample family
  // that isn't Ender, live as the person types - the whole point is to make
  // it obvious when a phrase doesn't actually match (a typo, wrong
  // punctuation, names in the wrong order) instead of silently saving flat
  // text that renders identically for every family, which is exactly what
  // happened before this preview existed.
  const previewValue = el('span', 'gv-new-greeting-preview-value');
  const preview = el('div', 'gv-new-greeting-preview');
  preview.append(el('div', 'gv-new-greeting-preview-label', 'Preview'), previewValue);
  const previewNote = el('div', 'gv-new-greeting-preview-note',
    'No phrase matched, so this will show exactly as typed for every family - fine for a fixed line, not if you meant to personalize it.');
  previewNote.hidden = true;
  function updatePreview() {
    const format = formatInput.value;
    const rendered = grouped
      ? buildGreeting(format, GREETING_PREVIEW_FAMILY)
      : buildGreeting(format, {person: GREETING_PREVIEW_FAMILY.adults[0]});
    previewValue.textContent = rendered || '(empty)';
    previewNote.hidden = rendered !== format;
  }
  formatInput.addEventListener('input', updatePreview);

  // Holds the Greeting field itself plus everything tied to it (preview,
  // hint, Cancel/Save) - hidden until there's actually something to edit, so
  // opening the dialog onto an existing list of greetings doesn't also throw
  // a blank editor at the person before they've picked (or asked for) one.
  const editorSection = el('div', 'gv-greeting-editor');

  // Loads an existing greeting into the field to edit it, or (passed null,
  // from Add another greeting) clears it back to a fresh default - either
  // way this is the one place that changes what Save will do, and what
  // reveals the editor in the first place.
  function loadGreeting(g) {
    original = g ? g.name : '';
    formatInput.value = g ? g.format : defaultGreetingFormat();
    cards.forEach(({card, greeting}) => card.classList.toggle('active', greeting === g));
    editorSection.hidden = false;
    updatePreview();
    formatInput.focus();
    formatInput.select();
  }

  const meEmail = document.body.dataset.userEmail;
  const mine = inviteGreetings.filter(g => g.createdBy && g.createdBy === meEmail);
  if (mine.length) {
    const listWrap = el('div', 'gv-greeting-list');
    listWrap.append(el('div', 'gv-greeting-list-title', 'Your greetings'));
    for (const g of mine) {
      const card = el('div', 'gv-greeting-card');
      card.append(el('span', 'gv-greeting-card-text', g.format));
      const cardActions = el('div', 'gv-greeting-card-actions');
      const editBtn = el('button', 'gv-greeting-card-btn');
      editBtn.type = 'button';
      editBtn.title = 'Edit';
      editBtn.append(svg('pencil'));
      editBtn.addEventListener('click', () => loadGreeting(g));
      const deleteBtn = el('button', 'gv-greeting-card-btn gv-greeting-card-btn-delete');
      deleteBtn.type = 'button';
      deleteBtn.title = 'Delete';
      deleteBtn.append(svg('trash'));
      deleteBtn.addEventListener('click', async () => {
        if (!confirm(`Delete "${g.format}"? This can't be undone.`)) {
          return;
        }
        error.hidden = true;
        deleteBtn.disabled = true;
        try {
          // As a query parameter, not a body - Go's ParseForm only reads a
          // request body for POST/PUT/PATCH, so a DELETE's body would
          // silently go unread server-side and every delete would 400 as
          // "missing name".
          const res = await fetch(`/api/directory/greetings?${new URLSearchParams({name: g.name})}`, {method: 'DELETE'});
          if (!res.ok) {
            throw new Error(await res.text());
          }
          inviteSystems = null;
          close();
          onSaved();
        } catch (err) {
          error.hidden = false;
          error.textContent = 'Couldn’t delete that greeting - try again.';
          deleteBtn.disabled = false;
        }
      });
      cardActions.append(editBtn, deleteBtn);
      card.append(cardActions);
      cards.push({card, greeting: g});
      listWrap.append(card);
    }
    const addButton = el('button', 'gv-greeting-add');
    addButton.type = 'button';
    const addIcon = el('span', 'gv-greeting-add-icon');
    addIcon.append(svg('plus'));
    addButton.append(addIcon, el('span', '', 'Add another greeting'));
    addButton.addEventListener('click', () => loadGreeting(null));
    listWrap.append(addButton);
    form.append(listWrap);
    // Lives inside editorSection (its top edge) rather than out here, so it
    // only shows once there's an editor below it to divide the list from.
    editorSection.append(el('div', 'gv-greeting-divider'));
  }

  formatInput.value = defaultGreetingFormat();
  editorSection.append(formatLabel);
  editorSection.append(preview, previewNote);
  updatePreview();

  // Same worked-example names buildGreeting recognizes - see GreetingTemplate
  // in invites.go for why this is examples, not {{ token }} syntax.
  const hint = el('div', 'gv-new-greeting-hint',
    'Use the family’s actual names (e.g., Pat, Quinn, Ali, Bo) in your own words.');
  editorSection.append(hint);

  const error = el('div', 'gv-new-greeting-error');
  error.hidden = true;
  editorSection.append(error);

  const actions = el('div', 'gv-new-greeting-actions');
  const cancel = el('button', 'gv-new-greeting-cancel', 'Cancel');
  cancel.type = 'button';
  cancel.addEventListener('click', close);
  const save = el('button', 'filter-done gv-new-greeting-save', 'Save');
  save.type = 'submit';
  actions.append(cancel, save);
  editorSection.append(actions);
  // Nothing to browse first (no custom greetings yet) - open straight into
  // the editor instead of showing an empty "Your greetings" list with
  // nothing but an Add button in it.
  editorSection.hidden = mine.length > 0;
  form.append(editorSection);

  form.addEventListener('submit', async e => {
    e.preventDefault();
    const format = formatInput.value.trim();
    if (!format) {
      return;
    }
    error.hidden = true;
    save.disabled = true;
    try {
      // Applies to whichever mode the picker this dialog was opened from is
      // currently showing, not asked for - Group by family vs. Individual is
      // already a choice the person made just above the table, and asking
      // them to repeat it here would just be one more thing to get wrong.
      const body = new URLSearchParams({
        format,
        original,
        grouped: state.gvInviteBy === 'group' ? '1' : '0',
        individual: state.gvInviteBy !== 'group' ? '1' : '0',
      });
      const res = await fetch('/api/directory/greetings', {method: 'POST', body});
      if (!res.ok) {
        throw new Error(await res.text());
      }
      // Cleared so the next loadInviteSystems call refetches instead of
      // serving the cached list, which is missing the row just changed.
      inviteSystems = null;
      state.gvGreeting = format;
      close();
      onSaved();
    } catch (err) {
      error.hidden = false;
      error.textContent = 'Couldn’t save that greeting - try again.';
      save.disabled = false;
    }
  });

  panel.append(form);
  overlay.append(panel);
  document.body.append(overlay);
  formatInput.focus();
  formatInput.select();
}

// A settings-bar segment: a label (with an optional info button explaining a
// less self-evident setting) above whatever control the caller appends next -
// a <select> for Export for, a switch for everything else.
function gvSegment(row, label, infoText) {
  const seg = el('div', 'gv-settings-segment');
  const labelRow = el('div', 'gv-setting-label');
  labelRow.append(el('span', '', label));
  if (infoText) {
    const info = el('button', 'gv-info-btn');
    info.type = 'button';
    info.title = infoText;
    info.setAttribute('aria-label', label + ': ' + infoText);
    info.append(svg('info'));
    labelRow.append(info);
  }
  seg.append(labelRow);
  row.append(seg);
  return seg;
}

function gvSwitch(seg, checked, onChange) {
  const toggle = el('input', 'filter-switch');
  toggle.type = 'checkbox';
  toggle.checked = checked;
  toggle.addEventListener('change', () => onChange(toggle.checked));
  seg.append(toggle);
  return toggle;
}

// One bar - Export for, then whichever of Group by family/Include siblings/
// Send to Kid Emails apply - divided into segments rather than separate
// cards, with the selected system's own description as a single summary line
// underneath instead of scattered per-control. onSystemChange rebuilds this
// whole bar (and the page around it) - needed whenever which system, or Group
// vs. Individual, changes, since those decide which segments even show and
// which greeting options apply (chosen in the table header - see renderGrid's
// greetingCol). onSettingChange just re-runs the grid, for Include siblings/
// Send to Kid Emails, which never change what's shown here.
function renderInviteSettings(systems, settings, onSystemChange, onSettingChange) {
  const system = systems.find(s => s.name === state.gvSystem) || systems[0];
  // A system with no group concept has no Group by family choice to offer -
  // it's always Individual, silently, rather than a switch that doesn't work.
  if (!system.supportsGroups) {
    state.gvInviteBy = 'individual';
  }
  // Keep state.gvGreeting valid for whichever formats currently apply (see
  // greetingFormatsFor) - switching Group by family or system can leave a
  // stale key selected that's no longer one of the current options.
  if (templateUsesGreeting(system.rows)) {
    const formats = greetingFormatsFor(state.gvInviteBy, system.supportsGroups);
    if (formats.length && !formats.some(f => f.name === state.gvGreeting)) {
      state.gvGreeting = formats[0].name;
    }
  }

  const bar = el('div', 'gv-settings-bar');
  const row = el('div', 'gv-settings-row');
  bar.append(row);

  const exportSeg = gvSegment(row, 'Export for');
  gvServiceSelect(exportSeg, systems, name => {
    const next = systems.find(s => s.name === name);
    state.gvSystem = name;
    // Picking a different system resets Group by family to that system's own
    // default (on if it supports one, otherwise off) rather than carrying
    // over whatever the previous system happened to be set to.
    state.gvInviteBy = next && next.supportsGroups ? 'group' : 'individual';
    onSystemChange();
  });

  // Hidden entirely for a system with no group concept (Punchbowl, Partiful) -
  // there's no Group option to offer, so it's always Individual, silently.
  if (system.supportsGroups) {
    row.append(el('div', 'gv-settings-divider'));
    const groupSeg = gvSegment(row, 'Group by family',
      'On: one invite per family, addressed to everyone in the household together. Off: one invite per person.');
    gvSwitch(groupSeg, state.gvInviteBy === 'group', checked => {
      state.gvInviteBy = checked ? 'group' : 'individual';
      onSystemChange();
    });
  }

  row.append(el('div', 'gv-settings-divider'));
  const siblingsSeg = gvSegment(row, 'Include siblings', 'Will include the children or siblings of invitees.');
  gvSwitch(siblingsSeg, state.gvSiblings, checked => {
    state.gvSiblings = checked;
    onSettingChange();
  });

  // Only meaningful in Group mode, where it decides how a sibling appears in
  // the family's member columns (name only, or name plus their own email). In
  // Individual mode a student is never emailed directly at all - see
  // personInviteParams - so there's nothing here for this switch to control.
  if (state.gvInviteBy === 'group') {
    row.append(el('div', 'gv-settings-divider'));
    const emailSeg = gvSegment(row, 'Send to Kid Emails', 'Will include the student emails of the invitees.');
    gvSwitch(emailSeg, state.gvKidEmail, checked => {
      state.gvKidEmail = checked;
      onSettingChange();
    });
  }

  if (system.description) {
    bar.append(el('div', 'gv-fyi gv-settings-summary', system.description));
  }
  settings.append(bar);
}
