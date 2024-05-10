package source

import (
	"net"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
)

type HookStore map[string]Hook

type Hook func(HookParam) bool

type HookParam struct {
	IP    *string
	Label map[string]string
}

func HookParamWithIP(ip string) func(*HookParam) {
	return func(p *HookParam) {
		p.IP = &ip
	}
}

func HookParamWithLabels(lbl map[string]string) func(*HookParam) {
	return func(p *HookParam) {
		p.Label = lbl
	}
}

func NewHookParam(opts ...func(*HookParam)) HookParam {
	p := HookParam{}
	for _, opt := range opts {
		opt(&p)
	}
	return p
}

type orHooks []hook

func (s orHooks) Select(p HookParam) bool {
	for idx := range s {
		if s[idx].Select(p) {
			return true
		}
	}
	return false
}

type hook struct {
	labelSelectHook
	ipSelectHook
}

func (h hook) Select(p HookParam) bool {
	if p.Label != nil && h.labelSelectHook != nil && !h.labelSelectHook(p.Label) {
		return false
	}
	if p.IP != nil && h.ipSelectHook != nil && !h.ipSelectHook(*p.IP) {
		return false
	}
	return true
}

type IPSelector struct {
	IncludeIP bool
	IPs       []string
	CIDRs     []string
}

// Selector is used to build a single hook
// The relationship between IPSelectors in the list is ANDed
// The relationship between LabelSelector and IPSelectors list is ANDed
type Selector struct {
	LabelSelector *metav1.LabelSelector
	IPSelectors   []*IPSelector
}

// HookConfig is used to build Hook
// The relationship between Selectors is ORed
type HookConfig struct {
	// EmptySelectorsReturn is returned when both LabelSelector and IPSelectors of a Selector are empty,
	// or the Selectors list is empty
	EmptySelectorsReturn bool

	Selectors []*Selector
}

func HookConfigWithEmptySelectorsReturn(emptySelectorsReturn bool) func(*HookConfig) {
	return func(cfg *HookConfig) {
		cfg.EmptySelectorsReturn = emptySelectorsReturn
	}
}

func NewHookStore(cfgs map[string]HookConfig) HookStore {
	m := make(map[string]Hook, len(cfgs))
	for key, cfg := range cfgs {
		if len(cfg.Selectors) == 0 {
			m[key] = func(_ HookParam) bool { return cfg.EmptySelectorsReturn }
		} else {
			var hooks orHooks
			for _, selector := range cfg.Selectors {
				h := hook{}
				// should not happen
				if len(selector.IPSelectors) == 0 && selector.LabelSelector == nil {
					// only set one of the labelSelectHook and ipSelectHook
					h.labelSelectHook = func(_ map[string]string) bool { return cfg.EmptySelectorsReturn }
				} else {
					if selector.LabelSelector != nil {
						h.labelSelectHook = newLabelSelectHook([]*metav1.LabelSelector{selector.LabelSelector}, cfg.EmptySelectorsReturn)
					}
					if len(selector.IPSelectors) != 0 {
						h.ipSelectHook = newIPSelectHook(selector.IPSelectors, cfg.EmptySelectorsReturn)
					}
				}
				hooks = append(hooks, h)
			}
			m[key] = hooks.Select
		}
	}
	return m
}

// labelSelectHook returns TRUE if matched
type labelSelectHook func(map[string]string) bool

// newLabelSelectHook build a SelectHook by the input LabelSelectors.
// If the input LabelSelectors is nil, the returned hook returns
// the emptySelectorsReturn.
// The relationship between labelSelectors in the list is ORed
func newLabelSelectHook(labelSelectors []*metav1.LabelSelector, emptySelectorsReturn bool) labelSelectHook {
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

// ipSelectHook returns TRUE if matched
type ipSelectHook func(string) bool

// newIPSelectHook builds an ipSelectHook by the input config list.
// If the input config list is nil, the returned hook returns
// the emptySelectorsReturn.
// The relationship between cfg in the cfgs is ANDed
func newIPSelectHook(cfgs []*IPSelector, emptySelectorsReturn bool) ipSelectHook {
	if len(cfgs) == 0 {
		return func(_ string) bool { return emptySelectorsReturn }
	}
	hooks := make([]ipSelectHook, 0, len(cfgs))
	for _, cfg := range cfgs {
		hooks = append(hooks, singleIPSelectHook(cfg, emptySelectorsReturn))
	}

	return func(inputIP string) bool {
		// should not happen
		if len(hooks) == 0 {
			return emptySelectorsReturn
		}
		for _, hook := range hooks {
			if !hook(inputIP) {
				return false
			}
		}
		return true
	}
}

// singleIPSelectHook builds an ipSelectHook by the input config.
// If the input IPs and CIDRs is nil, the returned hook returns
// the emptySelectorsReturn.
// If at least one of the input IPs and CIDRs is not nil, the input IP
// returns cfg.IncludeIP when it is in IPs or CIDRs.
func singleIPSelectHook(cfg *IPSelector, emptySelectorsReturn bool) ipSelectHook {
	if cfg == nil ||
		(len(cfg.IPs) == 0 && len(cfg.CIDRs) == 0) {
		return func(_ string) bool { return emptySelectorsReturn }
	}

	parsedCidrs := make([]*net.IPNet, 0, len(cfg.CIDRs))
	for _, cidr := range cfg.CIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			// ignore invalid CIDR
			continue
		}
		parsedCidrs = append(parsedCidrs, ipNet)
	}
	clonedIPs := append([]string{}, cfg.IPs...)
	return func(inputIP string) bool {
		// should not happen
		if len(parsedCidrs) == 0 && len(clonedIPs) == 0 {
			return emptySelectorsReturn
		}
		for _, ipNet := range parsedCidrs {
			if ipNet.Contains(net.ParseIP(inputIP)) {
				return cfg.IncludeIP
			}
		}
		for _, ip := range clonedIPs {
			if inputIP == ip {
				return cfg.IncludeIP
			}
		}
		return !cfg.IncludeIP
	}
}

func ConvertEndpointSelectorToHookConfig(sels []*bootstrap.EndpointSelector, opts ...func(*HookConfig)) HookConfig {
	list := make([]*Selector, 0, len(sels))
	for _, sel := range sels {
		var cfg Selector
		if sel.LabelSelector != nil {
			cfg.LabelSelector = sel.LabelSelector
		}
		if sel.ExcludeIPRanges != nil {
			ipSel := IPSelector{IncludeIP: false}
			if len(sel.ExcludeIPRanges.IPs) != 0 {
				ipSel.IPs = append(ipSel.IPs, sel.ExcludeIPRanges.IPs...)
			}
			if len(sel.ExcludeIPRanges.CIDRs) != 0 {
				ipSel.CIDRs = append(ipSel.CIDRs, sel.ExcludeIPRanges.CIDRs...)
			}
			if len(ipSel.IPs) != 0 || len(ipSel.CIDRs) != 0 {
				cfg.IPSelectors = append(cfg.IPSelectors, &ipSel)
			}
		}
		if cfg.LabelSelector != nil || len(cfg.IPSelectors) > 0 {
			list = append(list, &cfg)
		}
	}
	ret := HookConfig{
		Selectors: list,
	}

	for _, opt := range opts {
		opt(&ret)
	}

	return ret
}
