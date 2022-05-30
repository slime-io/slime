package model

import (
	"github.com/sirupsen/logrus"
	frameworkmodel "slime.io/slime/framework/model"
)

const ModuleName = "plugin"

var ModuleLog = logrus.WithField(frameworkmodel.LogFieldKeyModule, ModuleName)
