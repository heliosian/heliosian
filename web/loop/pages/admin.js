import {me, isAdmin} from '../state.js';
import {adminPage as buildAdminPage, adminsCard} from '/admin.js';

const sections = [
  {title: 'Editing & Control', tabs: [
    {key: 'admins', label: 'Admins', card: () => adminsCard({hint: 'Whoever is on this list sees and can change every email list, not only the ones they manage, and reaches this page. Anyone signed in can make an email list of their own.'})},
  ]},
];

export function adminPage() {
  return buildAdminPage({appName: 'Helios Loop', allowed: isAdmin(), email: me().email, sections});
}
