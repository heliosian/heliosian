package testkit

func DeckRevisions() []map[string]string {
	return []map[string]string{
		{"id": "doc00000000901", "kind": "file", "name": "Gradeband week one", "superseded_by": "doc00000000902"},
		{"id": "doc00000000902", "kind": "file", "name": "Gradeband week two", "superseded_by": "doc00000000903"},
		{"id": "doc00000000903", "kind": "file", "name": "Gradeband week three", "author": "per00000000003", "url": "https://example.org/current-deck.pdf"},
		{"id": "doc00000000904", "relation": "extract", "parent": "doc00000000901", "content": "cnt00000000101"},
	}
}
