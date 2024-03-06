package controllers

import (
	"slime.io/slime/framework/monitoring"
	"slime.io/slime/modules/lazyload/model"
)

var (
	SidecarCreations = monitoring.NewSum(
		model.ModuleName,
		"sidecar_creations",
		"total number of sidecar creations",
	)

	SidecarFailedCreations = monitoring.NewSum(
		model.ModuleName,
		"sidecar_failed_creations",
		"total number of sidecar creations failed",
	)

	SidecarRefreshes = monitoring.NewSum(
		model.ModuleName,
		"sidecar_refreshes",
		"total number of sidecar refreshes",
	)

	ServiceFenceCreations = monitoring.NewSum(
		model.ModuleName,
		"servicefence_creations",
		"total number of servicefence creations success",
	)

	ServiceFenceFailedCreations = monitoring.NewSum(
		model.ModuleName,
		"servicefence_failed_creations",
		"total number of servicefence creations failed",
	)

	ServiceFenceRefresh = monitoring.NewSum(
		model.ModuleName,
		"servicefence_refreshes",
		"total number of servicefence refreshes",
	)

	ServiceFenceDelections = monitoring.NewSum(
		model.ModuleName,
		"servicefence_deletions",
		"total number of servicefence deletions",
	)

	UpdateExtraResourceFailed = monitoring.NewSum(
		model.ModuleName,
		"update_extra_resource_failed",
		"total number of update extra resource failed",
	)

	ServicefenceLoads = monitoring.NewHistogram(
		model.ModuleName,
		"servicefenc_loads",
		"total number of servicefence loads",
	)
)
