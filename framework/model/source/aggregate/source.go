package aggregate

import (
	"k8s.io/apimachinery/pkg/types"
	"slime.io/slime/framework/model/source"
)

type Source struct {
	Sources []source.Source
}

func (s *Source) Start(stop <-chan struct{}) {
	for _, v := range s.Sources {
		v.Start(stop)
	}
}

func (s *Source) WatchAdd(meta types.NamespacedName) {
	for _, v := range s.Sources {
		v.WatchAdd(meta)
	}
}

func (s *Source) WatchRemove(meta types.NamespacedName) {
	for _, v := range s.Sources {
		v.WatchRemove(meta)
	}
}

func (s *Source) Get(meta types.NamespacedName) map[string]string {
	aggregateMap := make(map[string]string)
	for _, v := range s.Sources {
		for k, v := range v.Get(meta) {
			aggregateMap[k] = v
		}
	}
	return aggregateMap
}

func (s *Source) AppendSource(src source.Source) {
	s.Sources = append(s.Sources, src)
}
