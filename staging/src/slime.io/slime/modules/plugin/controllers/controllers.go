package controllers

import (
	"k8s.io/apimachinery/pkg/types"
	frameworkmodel "slime.io/slime/framework/model"
	"slime.io/slime/modules/plugin/model"
)

var (
	log     = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "controllers")
	emptyNN = types.NamespacedName{}
)
