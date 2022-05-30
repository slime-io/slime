package model

import (
	"github.com/sirupsen/logrus"
	frameworkmodel "slime.io/slime/framework/model"
)

const ModuleName = "limiter"

var ModuleLog = logrus.WithField(frameworkmodel.LogFieldKeyModule, ModuleName)

type RateLimitConfig struct {
	Domain      string        `yaml:"domain,omitempty"`
	Descriptors []*Descriptor `yaml:"descriptors,omitempty"`
}

type Descriptor struct {
	Key       string     `yaml:"key,omitempty"`
	Value     string     `yaml:"value,omitempty"`
	RateLimit *RateLimit `yaml:"rate_limit,omitempty"`
}

type RateLimit struct {
	RequestsPerUnit uint32 `yaml:"requests_per_unit,omitempty"`
	Unit            string `yaml:"unit,omitempty"`
}
