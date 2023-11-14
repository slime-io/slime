package controllers

import (
	"slime.io/slime/framework/monitoring"
	"slime.io/slime/modules/limiter/model"
)

var (
	ReconcilesTotal = monitoring.NewSum(
		model.ModuleName,
		"reconciles",
		"total number of smartlimiter reconciles",
	)

	ValidationFailedTotal = monitoring.NewSum(
		model.ModuleName,
		"validations_failed",
		"the total number of failed smartlimiter validations",
	)

	EnvoyFilterCreations = monitoring.NewSum(
		model.ModuleName,
		"envoyfilter_creations",
		"total number of envoyfilter creations",
	)

	EnvoyFilterCreationsFailed = monitoring.NewSum(
		model.ModuleName,
		"envoyfilter_creations_failed",
		"total number of envoyfilter creations failed",
	)

	EnvoyfilterRefreshes = monitoring.NewSum(
		model.ModuleName,
		"envoyfilter_refreshes",
		"total number of envoyfilter refreshes",
	)

	EnvoyfilterDeletions = monitoring.NewSum(
		model.ModuleName,
		"envoyfilter_deletions",
		"total number of envoyfilter deletions",
	)

	CachedLimiter = monitoring.NewGauge(
		model.ModuleName,
		"cached_limiter",
		"the number of cached limiter",
	)
)
