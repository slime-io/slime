package controllers

import (
	"slime.io/slime/framework/monitoring"
	"slime.io/slime/modules/plugin/model"
)

var (
	resourceName = monitoring.MustCreateLabel("resourceName")

	EnvoypluginReconciles = monitoring.NewSum(
		model.ModuleName,
		"envoyplugin_reconciles",
		"total number of envoyplugin reconciles",
	)

	EnvoypluginReconcilesFailed = monitoring.NewSum(
		model.ModuleName,
		"envoyplugin_reconciles_failed",
		"total number of envoyplugin reconciles failed",
	)

	PluginManagerReconciles = monitoring.NewSum(
		model.ModuleName,
		"pluginmanager_reconciles",
		"total number of pluginmanager reconciles",
	)

	PluginManagerReconcilesFailed = monitoring.NewSum(
		model.ModuleName,
		"pluginmanager_reconciles_failed",
		"total number of pluginmanager reconciles failed",
	)

	EnvoyfilterCreations = monitoring.NewSum(
		model.ModuleName,
		"envoyfilter_creations",
		"total number of envoyfilter creations",
	)

	EnvoyfilterCreationsFailed = monitoring.NewSum(
		model.ModuleName,
		"envoyfilter_creations_failed",
		"total number of envoyfilter creations failed",
	)

	EnvoyfilterRefreshes = monitoring.NewSum(
		model.ModuleName,
		"envoyfilter_refreshes",
		"total number of envoyfilter refreshes",
	)
)
