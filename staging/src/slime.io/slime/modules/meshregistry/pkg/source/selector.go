package source

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type SelectHookStore map[string]SelectHook

// SelectHook returns TRUE if matched
type SelectHook func(map[string]string) bool

// NewSelectHook build a SelectHook by the input LabelSelectors.
// If the input LabelSelectors is nil, the returned hook returns
// the emptySelectorsReturn.
func NewSelectHook(labelSelectors []*metav1.LabelSelector, emptySelectorsReturn bool) SelectHook {
	if len(labelSelectors) == 0 {
		return func(_ map[string]string) bool { return emptySelectorsReturn }
	}
	var selectors []labels.Selector
	for _, selector := range labelSelectors {
		ls, err := metav1.LabelSelectorAsSelector(selector)
		if err != nil {
			// ignore invalid LabelSelector
			continue
		}
		selectors = append(selectors, ls)
	}
	return func(m map[string]string) bool {
		if len(selectors) == 0 {
			return true
		}
		for _, selector := range selectors {
			if selector.Matches(labels.Set(m)) {
				return true
			}
		}
		return false
	}
}

// NewSelectHookStore returns a SelectHookStore
func NewSelectHookStore(groupedSelectors map[string][]*metav1.LabelSelector, emptySelectorsReturn bool) SelectHookStore {
	m := make(map[string]SelectHook, len(groupedSelectors))
	for key, sels := range groupedSelectors {
		m[key] = NewSelectHook(sels, emptySelectorsReturn)
	}
	return m
}
