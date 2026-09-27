const known = {'Regular': 'dt-regular', 'No School': 'dt-no-school', 'Early Dismissal': 'dt-early', 'No Aftercare': 'dt-no-aftercare'};

export function dayTypeClass(name) {
  return known[name] || 'dt-other';
}
