import {initChrome} from './chrome.js';
import {showPage, clearSearch} from '/shell.js';
import {initModal} from '/modal.js';
import {startApp, load, notFound} from '/router.js';
import {state, loadModel, me, gradebands, answerAsMe, findInvite} from './state.js';
import {dayWords, hourWords} from '/invitation.js';
import {homePage} from './pages/home.js';
import {invitePage} from './pages/invite.js';
import {setGuestHost} from '/guesthost.js';
import {imageTools} from '/images.js';
import {sidebarIn} from '/themes.js';
import {discardEditorOpen} from './editor.js';

const dayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'short', month: 'short', day: 'numeric'});

setGuestHost({
  me,
  answer: answerAsMe,
  isParty: () => false,
  gradebands,
  dayLabel: date => dayFormat.format(new Date(date + 'T12:00')),
  uploadImage: imageTools('/api/when').uploadImage,
  gradeColors: () => ({}),
  classroomColors: () => Object.fromEntries(state.model.classrooms.map(c => [c.name, c.color])),
  everyone: () => state.model.everyone,
  whenWords: e => {
    const inv = findInvite(e.id);
    const day = dayWords(inv);
    const hours = hourWords(inv);
    return {day, when: hours ? `${day}, ${hours}` : day};
  },
});

initChrome();
initModal(load);
startApp({
  model: loadModel,
  routes: {
    '': () => homePage(),
    g: parts => (parts[1] ? invitePage(parts[1], parts[2] || '', parts[3] || '') : notFound('That invitation')),
  },
  missing: 'is not on Helios Gather.',
  prepare: entered => {
    sidebarIn(null);
    clearSearch();
    if (entered) {
      discardEditorOpen();
    }
  },
  show: showPage,
});
