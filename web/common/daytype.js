// dayTypeClass is the colour a day type wears wherever a day is shown - the
// calendar's month and day words, Heliosian's rail - so a type named in two
// apps is painted the same without either holding its own list. A type the
// sheet has added since is dt-other, which every app styles.
const known = {'Regular': 'dt-regular', 'No School': 'dt-no-school', 'Early Dismissal': 'dt-early', 'No Aftercare': 'dt-no-aftercare'};

export function dayTypeClass(name) {
  return known[name] || 'dt-other';
}
