package db

import (
	"hash/fnv"
	"maps"
	"math"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode"
)

const (
	searchProbe   = 16
	searchTrain   = 4096
	searchRounds  = 8
	searchShingle = 5
	searchHashes  = 128
	searchBand    = 4
	searchSame    = 0.7
)

type searchView struct {
	rows    map[string]SearchRow
	objects map[string][]string
	entries map[string]*SearchEntry
	words   map[string][]string
	vectors map[string]*vectorIndex
	groups  map[string]string
	members map[string][]string
}

func emptyView() *searchView {
	vectors := map[string]*vectorIndex{}
	for _, t := range searchTables {
		vectors[t] = &vectorIndex{}
	}
	return &searchView{rows: map[string]SearchRow{}, objects: map[string][]string{}, entries: map[string]*SearchEntry{}, words: map[string][]string{}, vectors: vectors, groups: map[string]string{}, members: map[string][]string{}}
}

func buildView(rows map[string]SearchRow, entries map[string]*SearchEntry, vectors map[string]*vectorIndex) *searchView {
	v := &searchView{rows: rows, objects: map[string][]string{}, entries: entries, words: map[string][]string{}, vectors: vectors}
	for _, id := range slices.Sorted(maps.Keys(rows)) {
		if o := rows[id].Object; entries[o] != nil {
			v.objects[o] = append(v.objects[o], id)
		}
	}
	prints := map[string][]uint32{}
	for _, o := range slices.Sorted(maps.Keys(entries)) {
		e := entries[o]
		seen := map[string]bool{}
		for _, k := range e.Keywords {
			for _, w := range searchTerms(k) {
				if !seen[w] {
					seen[w] = true
					v.words[w] = append(v.words[w], o)
				}
			}
		}
		if len(e.Fingerprint) == searchHashes {
			prints[o] = e.Fingerprint
		}
	}
	v.groups, v.members = duplicates(prints)
	return v
}

func (v *searchView) group(object string) string {
	if g, ok := v.groups[object]; ok {
		return g
	}
	return object
}

func wordForms(t string) []string {
	out := []string{t, t + "s"}
	if trimmed := strings.TrimSuffix(t, "s"); trimmed != t && len(trimmed) > 1 {
		out = append(out, trimmed)
	}
	return out
}

func (v *searchView) byWords(words string) map[string]float64 {
	scores := map[string]float64{}
	total := 0.0
	for _, t := range searchTerms(words) {
		held := map[string]bool{}
		for _, form := range wordForms(t) {
			for _, o := range v.words[form] {
				held[o] = true
			}
		}
		if len(held) == 0 {
			continue
		}
		weight := math.Log(1 + float64(len(v.entries))/float64(len(held)))
		total += weight
		for o := range held {
			scores[o] += weight
		}
	}
	best := 0.0
	for o := range scores {
		scores[o] /= total
		best = max(best, scores[o])
	}
	for o, share := range scores {
		if share < searchWordShare*best {
			delete(scores, o)
		}
	}
	return scores
}

type chunkRef struct {
	object string
	vector []float32
}

type placement struct {
	entry *SearchEntry
	cells []int
}

type vectorIndex struct {
	centroids [][]float32
	cells     [][]chunkRef
	placed    map[string]placement
	trained   int
}

func (v *vectorIndex) update(entries map[string]*SearchEntry) *vectorIndex {
	refs := []chunkRef{}
	for _, o := range slices.Sorted(maps.Keys(entries)) {
		for _, c := range entries[o].Chunks {
			refs = append(refs, chunkRef{object: o, vector: c.Vector})
		}
	}
	if len(refs) == 0 {
		return &vectorIndex{}
	}
	if len(v.centroids) == 0 || len(refs) > 2*v.trained {
		return train(entries, refs)
	}
	out := &vectorIndex{centroids: v.centroids, cells: make([][]chunkRef, len(v.centroids)), placed: map[string]placement{}, trained: v.trained}
	for _, o := range slices.Sorted(maps.Keys(entries)) {
		e := entries[o]
		p, ok := v.placed[o]
		if !ok || p.entry != e {
			p = placement{entry: e, cells: make([]int, len(e.Chunks))}
			for i, c := range e.Chunks {
				p.cells[i] = nearest(out.centroids, c.Vector)
			}
		}
		out.placed[o] = p
		for i, c := range e.Chunks {
			out.cells[p.cells[i]] = append(out.cells[p.cells[i]], chunkRef{object: o, vector: c.Vector})
		}
	}
	return out
}

func train(entries map[string]*SearchEntry, refs []chunkRef) *vectorIndex {
	k := max(1, int(math.Sqrt(float64(len(refs)))))
	sample := [][]float32{}
	step := max(1, len(refs)/searchTrain)
	for i := 0; i < len(refs); i += step {
		sample = append(sample, refs[i].vector)
	}
	centroids := [][]float32{}
	for i := range k {
		centroids = append(centroids, slices.Clone(sample[i*len(sample)/k]))
	}
	assigned := make([]int, len(sample))
	for range searchRounds {
		parallel(len(sample), func(i int) { assigned[i] = nearest(centroids, sample[i]) })
		sums := make([][]float32, k)
		for i, c := range assigned {
			if sums[c] == nil {
				sums[c] = make([]float32, len(sample[i]))
			}
			for j, f := range sample[i] {
				sums[c][j] += f
			}
		}
		for c, s := range sums {
			if s != nil {
				normalize(s)
				centroids[c] = s
			}
		}
	}
	out := &vectorIndex{centroids: centroids, cells: make([][]chunkRef, k), placed: map[string]placement{}, trained: len(refs)}
	where := make([]int, len(refs))
	parallel(len(refs), func(i int) { where[i] = nearest(centroids, refs[i].vector) })
	at := 0
	for _, o := range slices.Sorted(maps.Keys(entries)) {
		e := entries[o]
		p := placement{entry: e, cells: make([]int, len(e.Chunks))}
		for i := range e.Chunks {
			p.cells[i] = where[at]
			out.cells[where[at]] = append(out.cells[where[at]], refs[at])
			at++
		}
		out.placed[o] = p
	}
	return out
}

func (v *vectorIndex) order(query []float32) []int {
	order := make([]int, len(v.centroids))
	near := make([]float32, len(v.centroids))
	for i, c := range v.centroids {
		order[i], near[i] = i, dot(query, c)
	}
	slices.SortFunc(order, func(a, b int) int { return cmpDesc(near[a], near[b]) })
	return order
}

func (v *vectorIndex) scan(query []float32, cells []int, best map[string]float64) {
	for _, c := range cells {
		for _, ref := range v.cells[c] {
			d := float64(dot(query, ref.vector))
			if old, ok := best[ref.object]; !ok || d > old {
				best[ref.object] = d
			}
		}
	}
}

func cmpDesc[T float32 | float64](a, b T) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	}
	return 0
}

func nearest(centroids [][]float32, v []float32) int {
	best, at := float32(math.Inf(-1)), 0
	for i, c := range centroids {
		if d := dot(v, c); d > best {
			best, at = d, i
		}
	}
	return at
}

func parallel(n int, f func(i int)) {
	workers := min(n, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := w; i < n; i += workers {
				f(i)
			}
		})
	}
	wg.Wait()
}

func dot(a, b []float32) float32 {
	n := min(len(a), len(b))
	var s0, s1, s2, s3 float32
	i := 0
	for ; i+4 <= n; i += 4 {
		s0 += a[i] * b[i]
		s1 += a[i+1] * b[i+1]
		s2 += a[i+2] * b[i+2]
		s3 += a[i+3] * b[i+3]
	}
	for ; i < n; i++ {
		s0 += a[i] * b[i]
	}
	return s0 + s1 + s2 + s3
}

var searchSeeds = func() []uint64 {
	out := make([]uint64, searchHashes)
	for i := range out {
		out[i] = mix(uint64(i) + 1)
	}
	return out
}()

func mix(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func fingerprint(text string) []uint32 {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !wordRune(r) })
	if len(words) == 0 {
		return nil
	}
	out := make([]uint32, searchHashes)
	for i := range out {
		out[i] = math.MaxUint32
	}
	for i := range max(1, len(words)-searchShingle+1) {
		h := fnv.New64a()
		h.Write([]byte(strings.Join(words[i:min(len(words), i+searchShingle)], " ")))
		x := h.Sum64()
		for j, seed := range searchSeeds {
			out[j] = min(out[j], uint32(mix(x^seed)>>32))
		}
	}
	return out
}

func resemblance(a, b []uint32) float64 {
	same := 0
	for i := range a {
		if a[i] == b[i] {
			same++
		}
	}
	return float64(same) / float64(len(a))
}

func duplicates(prints map[string][]uint32) (map[string]string, map[string][]string) {
	parent := map[string]string{}
	var find func(o string) string
	find = func(o string) string {
		p, ok := parent[o]
		if !ok || p == o {
			return o
		}
		root := find(p)
		parent[o] = root
		return root
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		if rb < ra {
			ra, rb = rb, ra
		}
		parent[rb] = ra
	}
	buckets := map[uint64][]string{}
	for _, o := range slices.Sorted(maps.Keys(prints)) {
		p := prints[o]
		for band := 0; band < searchHashes/searchBand; band++ {
			h := fnv.New64a()
			for _, x := range p[band*searchBand : (band+1)*searchBand] {
				h.Write([]byte{byte(x), byte(x >> 8), byte(x >> 16), byte(x >> 24)})
			}
			key := mix(h.Sum64() ^ uint64(band))
			for _, other := range buckets[key] {
				if find(other) != find(o) && resemblance(p, prints[other]) >= searchSame {
					union(o, other)
				}
			}
			buckets[key] = append(buckets[key], o)
		}
	}
	groups, members := map[string]string{}, map[string][]string{}
	for _, o := range slices.Sorted(maps.Keys(prints)) {
		g := find(o)
		groups[o] = g
		members[g] = append(members[g], o)
	}
	for g, m := range members {
		if len(m) == 1 {
			delete(members, g)
			delete(groups, m[0])
		}
	}
	return groups, members
}

func wordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
