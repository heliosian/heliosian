package db

func (s *sexp) equal(o *sexp) bool {
	if s.isList != o.isList {
		return false
	}
	if !s.isList {
		return s.kind == o.kind && s.text == o.text
	}
	if len(s.list) != len(o.list) {
		return false
	}
	for i := range s.list {
		if !s.list[i].equal(o.list[i]) {
			return false
		}
	}
	return true
}
