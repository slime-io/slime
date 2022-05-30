package controllers

import (
	"regexp"

	lazyloadv1alpha1 "slime.io/slime/modules/lazyload/api/v1alpha1"
)

func newDomainAliasRules(domainAlias []*lazyloadv1alpha1.DomainAlias) []*domainAliasRule {
	var rules []*domainAliasRule
	if domainAlias == nil {
		return nil
	}

	for _, da := range domainAlias {
		log.Infof("lazyload domainAlias: get pattern %s, get templates %v", da.Pattern, da.Templates)

		pattern := da.Pattern
		re, err := regexp.Compile(pattern)
		if err != nil {
			log.Errorf("compile domainAlias pattern %s err: %+v", pattern, err)
			return nil
		}
		templates := da.Templates
		if len(templates) == 0 {
			log.Errorf("domainAlias template is empty")
			return nil
		}
		rule := &domainAliasRule{
			pattern:   pattern,
			templates: templates,
			re:        re,
		}
		rules = append(rules, rule)
	}
	return rules
}

func domainAddAlias(src string, rules []*domainAliasRule) []string {
	dest := []string{src}
	if rules == nil {
		return dest
	}

	for _, rule := range rules {
		allIndexes := rule.re.FindAllSubmatchIndex([]byte(src), -1)
		if idxNum := len(allIndexes); idxNum != 1 {
			if idxNum > 1 {
				log.Warnf("domain %s matches more than once on pattern %s", src, rule.pattern)
			}
			continue
		}
		for _, template := range rule.templates {
			var domain []byte
			// expand the template according allIndexes
			domain = rule.re.ExpandString(domain, template, src, allIndexes[0])
			if len(domain) == 0 {
				continue
			}
			dest = append(dest, string(domain))
		}
	}

	log.Debugf("domainAddAlias src: %s, dest: %v", src, dest)
	return dest
}

func addToDomains(domains map[string]*lazyloadv1alpha1.Destinations, fh string) {
	if domains[fh] != nil {
		return
	}

	allHost := []string{fh}
	if hs := getDestination(fh); len(hs) > 0 {
		allHost = append(allHost, hs...)
	}

	domains[fh] = &lazyloadv1alpha1.Destinations{
		Hosts:  allHost,
		Status: lazyloadv1alpha1.Destinations_ACTIVE,
	}
}
