import {state, applyModel, staff, charity, isUnassigned, isAdmin, commsOnly} from './state.js';
import {initChrome} from './chrome.js';
import {showPage, clearSearch} from '/shell.js';
import {api} from '/api.js';
import {offerTeam} from './edit.js';
import {initModal} from '/modal.js';
import {startApp, load, notFound} from '/router.js';
import {jobsPage} from './pages/jobs.js';
import {processPage} from './pages/process.js';
import {calendarPage} from './pages/calendar.js';
import {staffPage} from './pages/staff.js';
import {charitiesPage, charityPage} from './pages/charities.js';
import {newslettersPage, newsletterPage} from './pages/newsletters.js';
import {skippedPage} from './pages/skipped.js';
import {unassignedPage} from './pages/unassigned.js';
import {adminPage} from './pages/admin.js';

const routes = {
  '': () => {
    if (commsOnly()) {
      return newslettersPage();
    }
    return state.model.staff.some(isUnassigned) ? unassignedPage() : jobsPage();
  },
  jobs: () => jobsPage(),
  process: () => processPage(),
  calendar: () => calendarPage(),
  charities: parts => {
    if (!parts[1]) {
      return charitiesPage();
    }
    const c = charity(parts[1]);
    return c ? charityPage(c) : notFound(parts[1]);
  },
  newsletters: parts => parts[1] && state.model.newsletterDates.includes(parts[1]) ? newsletterPage(parts[1]) : parts[1] ? notFound(parts[1]) : newslettersPage(),
  skipped: () => isAdmin() ? skippedPage() : notFound('That page'),
  unassigned: () => unassignedPage(),
  admin: () => adminPage(),
  staff: parts => {
    const sv = staff(parts[1]);
    return sv ? staffPage(sv) : notFound(parts[1] || 'That person');
  },
};

initChrome();
initModal(load);
startApp({
  model: async () => applyModel(await api('GET', '/api/birthday/model')),
  routes,
  missing: 'is not in the app.',
  prepare: clearSearch,
  show: showPage,
}).then(() => {
  if (location.pathname !== '/admin') {
    offerTeam();
  }
});
