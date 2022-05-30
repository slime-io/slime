/*
* @Author: yangdihang
* @Date: 2020/6/2
 */

package controllers

import (
	"regexp"
	"sync"

	"slime.io/slime/framework/model"
	modmodel "slime.io/slime/modules/lazyload/model"
)

var log = modmodel.ModuleLog.WithField(model.LogFieldKeyPkg, "controllers")

type Diff struct {
	Deleted []string
	Added   []string
}

type LabelItem struct {
	Name  string
	Value string
}

type NsSvcCache struct {
	Data map[string]map[string]struct{}
	sync.RWMutex
}

type LabelSvcCache struct {
	Data map[LabelItem]map[string]struct{}
	sync.RWMutex
}

type domainAliasRule struct {
	pattern   string
	templates []string
	re        *regexp.Regexp
}
