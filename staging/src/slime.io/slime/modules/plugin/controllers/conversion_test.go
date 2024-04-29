package controllers

import (
	"testing"

	"slime.io/slime/framework/util"
	"slime.io/slime/modules/plugin/api/v1alpha1"
)

func TestPluginManagerReconciler_getListenerFilterName(t *testing.T) {
	tests := []struct {
		name string
		in   *v1alpha1.Plugin
		want string
	}{
		{
			name: "http connection manager",
			in: &v1alpha1.Plugin{
				Protocol: v1alpha1.Plugin_HTTP,
			},
			want: util.EnvoyHTTPConnectionManager,
		},
		{
			name: "dubbo proxy",
			in: &v1alpha1.Plugin{
				Protocol: v1alpha1.Plugin_Dubbo,
			},
			want: util.EnvoyDubboProxy,
		},
		{
			name: "generic proxy",
			in: &v1alpha1.Plugin{
				Protocol:           v1alpha1.Plugin_Generic,
				GenericAppProtocol: "thrift",
			},
			want: util.EnvoyGenericProxyPrefix + "thrift",
		},
		{
			name: "unknown protocol",
			in: &v1alpha1.Plugin{
				Protocol: v1alpha1.Plugin_Protocol(100),
			},
			want: "",
		},
	}
	r := &PluginManagerReconciler{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.getListenerFilterName(tt.in); got != tt.want {
				t.Errorf("PluginManagerReconciler.getListenerFilterName() = %v, want %v", got, tt.want)
			}
		})
	}
}
