export function gradebandsOf(classrooms) {
  const out = [];
  for (const c of classrooms) {
    const name = c.gradeband || c.name;
    let band = out.find(b => b.name === name);
    if (!band) {
      band = {name, classrooms: []};
      out.push(band);
    }
    band.classrooms.push(c);
  }
  return out;
}
