package util

type Set[V comparable] map[V]struct{}

func NewSet[V comparable]() Set[V] {
	return make(Set[V])
}

func (s Set[V]) Insert(v V) {
	s[v] = struct{}{}
}

func (s Set[V]) Remove(v V) {
	delete(s, v)
}

func (s Set[V]) Contains(v V) bool {
	_, ok := s[v]
	return ok
}

func (s Set[V]) Len() int {
	return len(s)
}
