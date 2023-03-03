package source

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type SelectHookStore func(string) SelectHook

type ApplySelectHookStore func(SelectHookStore)

type SelectHook func(map[string]string) bool

type ApplyHook func(SelectHook)

// NewSelectHook build a SelectHook by the input LabelSelectors.
// If the input LabelSelectors is nil, the returned hook always returns TRUE.
func NewSelectHook(labelSelectors []*metav1.LabelSelector) SelectHook {
	if len(labelSelectors) == 0 {
		return func(_ map[string]string) bool { return true }
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

// UpdateSelector updates the given selector with the input LabelSelectors.
func UpdateSelector(labelSelectors []*metav1.LabelSelector, apply ApplyHook) {
	apply(NewSelectHook(labelSelectors))
}

// NewSelectHookStore returns a SelectHookStore
func NewSelectHookStore(groupedSelectors map[string][]*metav1.LabelSelector) SelectHookStore {
	m := make(map[string]SelectHook, len(groupedSelectors))
	for key, sels := range groupedSelectors {
		m[key] = NewSelectHook(sels)
	}
	return func(s string) SelectHook {
		return m[s]
	}
}

func UpdateGroupedSelector(groupedSelectors map[string][]*metav1.LabelSelector, apply ApplySelectHookStore) {
	apply(NewSelectHookStore(groupedSelectors))
}
