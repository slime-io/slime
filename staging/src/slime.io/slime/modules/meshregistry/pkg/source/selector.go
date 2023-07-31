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

type hook struct {
	SelectHook
	IPSelectHook
}

func (h hook) Select(p HookParam) bool {
	if p.Label != nil && h.SelectHook != nil && !h.SelectHook(p.Label) {
		return false
	}
	if p.IP != nil && h.IPSelectHook != nil && !h.IPSelectHook(*p.IP) {
		return false
	}
	return true
}

type IPSelector struct {
	IncludeIP bool
	IPs       []string
	CIDRs     []string
}

// HookConfig is used to build Hook
// The relationship between LabelSelectors in the list is OR
// The relationship between IPSelectors in the list is OR
// The relationship between LabelSelectors and IPSelectors is AND
// If LabelSelectors and IPSelectors are both empty, EmptySelectorsReturn is returned
type HookConfig struct {
	// EmptySelectorsReturn is returned when LabelSelectors, IPs, CIDRs are all empty
	EmptySelectorsReturn bool

	LabelSelectors []*metav1.LabelSelector

	IPSelectors []*IPSelector
}

func HookConfigWithEmptySelectorsReturn(emptySelectorsReturn bool) func(*HookConfig) {
	return func(cfg *HookConfig) {
		cfg.EmptySelectorsReturn = emptySelectorsReturn
	}
}

func NewHookStore(cfgs map[string]HookConfig) HookStore {
	m := make(map[string]Hook, len(cfgs))
	for key, cfg := range cfgs {
		h := hook{
			SelectHook:   NewSelectHook(cfg.LabelSelectors, cfg.EmptySelectorsReturn),
			IPSelectHook: NewIPSelectHook(cfg.IPSelectors, cfg.EmptySelectorsReturn),
		}
		m[key] = h.Select
	}
	return m
}

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

// IPSelectHook returns TRUE if matched
type IPSelectHook func(string) bool

// NewIPSelectHook builds an IPSelectHook by the input IPs and CIDRs.
// If the input IPs and CIDRs is nil, the returned hook returns
// the emptySelectorsReturn.
// If at least one of the input IPs and CIDRs is not nil, the input IP
// returns include when it is in IPs or CIDRs.
func NewIPSelectHook(cfgs []*IPSelector, emptySelectorsReturn bool) IPSelectHook {
	if len(cfgs) == 0 {
		return func(_ string) bool { return emptySelectorsReturn }
	}
	hooks := make([]IPSelectHook, 0, len(cfgs))
	for _, cfg := range cfgs {
		hooks = append(hooks, newIPSelectHook(cfg, emptySelectorsReturn))
	}

	return func(inputIP string) bool {
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

func newIPSelectHook(cfg *IPSelector, emptySelectorsReturn bool) IPSelectHook {
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
	labelSelectors := make([]*metav1.LabelSelector, 0, len(sels))
	ips := make([]string, 0, len(sels))
	cidrs := make([]string, 0, len(sels))
	for _, sel := range sels {
		if sel.LabelSelector != nil {
			labelSelectors = append(labelSelectors, sel.LabelSelector)
		}
		if len(sel.ExcludeIPRanges.IPs) != 0 {
			ips = append(ips, sel.ExcludeIPRanges.IPs...)
		}
		if len(sel.ExcludeIPRanges.CIDRs) != 0 {
			cidrs = append(cidrs, sel.ExcludeIPRanges.CIDRs...)
		}
	}
	cfg := HookConfig{
		LabelSelectors: labelSelectors,
		IPSelectors: []*IPSelector{
			{
				IncludeIP: false,
				IPs:       ips,
				CIDRs:     cidrs,
			},
		},
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return cfg
}
