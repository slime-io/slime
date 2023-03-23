package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	"sort"
	"strings"
	"sync"

	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"

	config "slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap"
	lazyloadconfig "slime.io/slime/modules/lazyload/api/config"
	"slime.io/slime/modules/lazyload/charts"
	"slime.io/slime/modules/lazyload/pkg/helm"
	"slime.io/slime/modules/lazyload/pkg/kube"
)

var (
	slimeBootGvr = schema.GroupVersionResource{
		Group:    "config.netease.com",
		Version:  "v1alpha1",
		Resource: "slimeboots",
	}

	defaultPort      = 80
	defaultProbePort = 18181
	defaultReplicas  = 1
	defaultIstioNs   = "istio-system"
	defaultSlimeNs   = "mesh-operator"

	renderOnce         sync.Once
	globalSidecarChart *chart.Chart
)

func loadGlobalSidecarChart() *chart.Chart {
	renderOnce.Do(func() {
		var err error
		globalSidecarChart, err = helm.LoadChartFromFS(charts.GlobalSidecarFS, charts.GlobalSidecar)
		if err != nil {
			log.Errorf("load global sidecar chart failed: %v", err)
		}
	})
	return globalSidecarChart
}

// TODO change these structs to framework

type SlimeBoot struct {
	metav1.TypeMeta   `json:",inline,omitempty" yaml:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec              *SlimeBootSpec `json:"spec" yaml:"spec"`
}

type SlimeBootSpec struct {
	Modules          []Module            `json:"module" yaml:"module"`
	Image            *Image              `json:"image" yaml:"image"`
	ImagePullSecrets []map[string]string `json:"imagePullSecrets" yaml:"imagePullSecrets"`
	Component        *Component          `json:"component" yaml:"component"`
	Namespace        string              `json:"namespace" yaml:"namespace"`
	IstioNamespace   string              `json:"istioNamespace" yaml:"istioNamespace"`
	Resources        *Resources          `json:"resources" yaml:"resources"`
}

type Component struct {
	GlobalSidecar *GlobalSidecar `json:"globalSidecar" yaml:"globalSidecar"`
}

type GlobalSidecar struct {
	Enable           bool           `json:"enable" yaml:"enable"`
	Image            *Image         `json:"image" yaml:"image"`
	Port             int            `json:"port" yaml:"port"`
	ProbePort        int            `json:"probePort" yaml:"probePort"`
	Replicas         int            `json:"replicas" yaml:"replicas"`
	Resources        *Resources     `json:"resources" yaml:"resources"`
	SidecarInject    *SidecarInject `json:"sidecarInject" yaml:"sidecarInject"`
	LegacyFilterName bool           `json:"legacyFilterName" yaml:"legacyFilterName"`
}

type SidecarInject struct {
	Enable      bool              `json:"enable" yaml:"enable"`
	Mode        string            `json:"mode" yaml:"mode"`
	Labels      map[string]string `json:"labels" yaml:"labels"`
	Annotations map[string]string `json:"annotations" yaml:"annotations"`
}

type Resources struct {
	Limits   *Limits   `json:"limits" yaml:"limits"`
	Requests *Requests `json:"requests" yaml:"requests"`
}

type Limits struct {
	CPU    string `json:"cpu" yaml:"cpu"`
	Memory string `json:"memory" yaml:"memory"`
}

type Requests struct {
	CPU    string `json:"cpu" yaml:"cpu"`
	Memory string `json:"memory" yaml:"memory"`
}

type Image struct {
	PullPolicy string `json:"pullPolicy" yaml:"pullPolicy"`
	Repository string `json:"repository" yaml:"repository"`
	Tag        string `json:"tag" yaml:"tag"`
}

type Module struct {
	Global  *config.Global        `protobuf:"bytes,3,opt,name=global,proto3" json:"global,omitempty"`
	Metric  *config.Metric        `protobuf:"bytes,6,opt,name=metric,proto3" json:"metric,omitempty"`
	Name    string                `protobuf:"bytes,5,opt,name=name,proto3" json:"name,omitempty"`
	Enable  bool                  `protobuf:"varint,7,opt,name=enable,proto3" json:"enable,omitempty"`
	General *lazyloadconfig.Fence `protobuf:"bytes,8,opt,name=general,proto3" json:"general,omitempty"`
	Bundle  *config.Bundle        `protobuf:"bytes,9,opt,name=bundle,proto3" json:"bundle,omitempty"`
	// like bundle item kind, necessary if not bundle
	Kind string `protobuf:"bytes,11,opt,name=kind,proto3" json:"kind,omitempty"`
}

func (module *Module) addDefaultValue() {
	if module.Global.IstioNamespace == "" {
		module.Global.IstioNamespace = defaultIstioNs
	}
	if module.Global.SlimeNamespace == "" {
		module.Global.SlimeNamespace = defaultSlimeNs
	}
	if module.Global.Misc == nil {
		module.Global.Misc = make(map[string]string)
	}
	module.Global.Misc["render"] = "lazyload"
}

func (slimeBoot *SlimeBoot) addDefaultValue() {
	if slimeBoot.Spec.Namespace == "" {
		slimeBoot.Spec.Namespace = defaultSlimeNs
	}
	if slimeBoot.Spec.IstioNamespace == "" {
		slimeBoot.Spec.IstioNamespace = defaultIstioNs
	}
	if slimeBoot.Spec.Component.GlobalSidecar.Port == 0 {
		slimeBoot.Spec.Component.GlobalSidecar.Port = defaultPort
	}
	if slimeBoot.Spec.Component.GlobalSidecar.ProbePort == 0 {
		slimeBoot.Spec.Component.GlobalSidecar.ProbePort = defaultProbePort
	}
	if slimeBoot.Spec.Component.GlobalSidecar.Replicas == 0 {
		slimeBoot.Spec.Component.GlobalSidecar.Replicas = defaultReplicas
	}
}

func updateResources(wormholePort []string, env *bootstrap.Environment) bool {
	log := log.WithField("function", "updateResources")
	dynCli := env.DynamicClient

	// chart
	chrt := loadGlobalSidecarChart()
	if chrt == nil {
		log.Errorf("can't load global sidecar chart")
		return false
	}

	// values
	owner, values, err := generateValuesFormSlimeboot(wormholePort, env)
	if err != nil {
		log.Errorf("generate values of global sidecar chart error: %v", err)
		return false
	}
	log.Debugf("got values %v to render global sider chart", values)

	// rander to generate new resources
	resources, err := generateNewReources(chrt, values)
	if err != nil {
		log.Errorf("generate new resources error: %v", err)
		return false
	}
	var ctx = context.Background()
	for gvr, resList := range resources {
		for _, res := range resList {
			ns, name := res.GetNamespace(), res.GetName()
			got, err := dynCli.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				if !errors.IsNotFound(err) {
					log.Errorf("got resource %s %s/%s from apiserver error: %v", gvr, ns, name, err)
					return false
				}
				// Setting ownerReferences before creation helps us clean up resources.
				// TODO:
				//   Resources located in other namespaces cannot be set ownerReferences,
				//   and we need other ways to clean up these resources.
				setOwnerReference(owner, res)
				_, err = dynCli.Resource(gvr).Namespace(ns).Create(ctx, res, metav1.CreateOptions{})
				if err != nil {
					log.Errorf("create resource %s %s/%s error: %v", gvr.String(), ns, name, err)
					return false
				}
				log.Infof("create resource %s %s/%s successfully", gvr.String(), ns, name)
			} else {
				obj := mergeObject(gvr, got, res)
				_, err = dynCli.Resource(gvr).Namespace(ns).Update(ctx, obj, metav1.UpdateOptions{})
				if err != nil {
					log.Errorf("update resource %s %s/%s error: %v", gvr, ns, name, err)
					return false
				}
				log.Infof("update resource %s %s/%s successfully", gvr.String(), ns, name)
			}
		}
	}
	return true
}

func generateValuesFormSlimeboot(wormholePort []string, env *bootstrap.Environment) (*SlimeBoot, map[string]interface{}, error) {
	slimeBoot, err := getSlimeboot(env)
	if err != nil {
		return nil, nil, fmt.Errorf("get slimeboot error: %v", err)
	}

	sort.Strings(wormholePort)
	log.Debugf("sorted wormholePort: %v", wormholePort)

	for idx, module := range slimeBoot.Spec.Modules {
		if module.Kind != "lazyload" {
			continue
		}
		slimeBoot.Spec.Modules[idx].General.WormholePort = wormholePort
		slimeBoot.Spec.Modules[idx].addDefaultValue()
	}
	slimeBoot.addDefaultValue()
	values, err := object2Values(slimeBoot.Spec)
	if err != nil {
		return nil, nil, fmt.Errorf("convert slimeboot spec to values error: %v", err)
	}
	return slimeBoot, values, nil
}

func getSlimeboot(env *bootstrap.Environment) (*SlimeBoot, error) {
	slimeBootNs := os.Getenv("WATCH_NAMESPACE")
	deployName := strings.Split(os.Getenv("POD_NAME"), "-")[0]

	utd, err := getSlimebootByOwnerRef(slimeBootNs, deployName, env)
	if err != nil {
		log.Infof("get slimeboot by ownerreferences failed with %q, try to get it by labelselector", err)
		utd, err = getSlimebootByLabelSelector(slimeBootNs, deployName, env)
		if err != nil {
			log.Infof("get slimeboot by labelselector failed with %q", err)
			return nil, fmt.Errorf("try to get slimeboot in namespace %s failed", slimeBootNs)
		}
	}

	// Unstructured -> SlimeBoot
	var slimeBoot SlimeBoot
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(utd.UnstructuredContent(), &slimeBoot); err != nil {
		return nil, fmt.Errorf("convert slimeboot %s/%s to structured error: %v", slimeBootNs, utd.GetName(), err)
	}

	return &slimeBoot, nil
}

func getSlimebootByOwnerRef(slimeBootNs, deployName string, env *bootstrap.Environment) (*unstructured.Unstructured, error) {
	kubeCli := env.K8SClient
	dynCli := env.DynamicClient

	// get slimeboot cr name
	deploy, err := kubeCli.AppsV1().Deployments(slimeBootNs).Get(context.TODO(), deployName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get lazyload deployment [%s/%s] error: %+v", slimeBootNs, deployName, err)
	}
	if len(deploy.OwnerReferences) == 0 {
		return nil, fmt.Errorf("lazyload deployment [%s/%s] does not have any ownerReferences", slimeBootNs, deployName)

	}
	slimeBootName := deploy.OwnerReferences[0].Name

	// Unstructured
	utd, err := dynCli.Resource(slimeBootGvr).Namespace(slimeBootNs).Get(context.TODO(), slimeBootName, metav1.GetOptions{}, "")
	if err != nil {
		return nil, fmt.Errorf("get slimeboot [%s/%s] error: %+v", slimeBootNs, slimeBootName, err)
	}

	return utd, nil
}

var slimebootSelectorTpl = "slime.io/slimeboot=%s"

func getSlimebootByLabelSelector(slimeBootNs, deployName string, env *bootstrap.Environment) (*unstructured.Unstructured, error) {
	dynCli := env.DynamicClient
	utdList, err := dynCli.Resource(slimeBootGvr).Namespace(slimeBootNs).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf(slimebootSelectorTpl, deployName),
	})
	if err != nil {
		return nil, fmt.Errorf("list slimeboot in %s error: %+v", slimeBootNs, err)
	}
	if utdList == nil || len(utdList.Items) == 0 {
		return nil, fmt.Errorf("could not find any slimeboot in namespace %s", slimeBootNs)
	}
	// By convention only one slimeboot will be matched to
	return &utdList.Items[0], nil
}

func object2Values(obj interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	err = json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func generateNewReources(chrt *chart.Chart, values map[string]interface{}) (map[schema.GroupVersionResource][]*unstructured.Unstructured, error) {
	manifests, err := helm.RenderChartWithValues(chrt, values)
	if err != nil {
		return nil, fmt.Errorf("render global sidecar chart with values error: %v", err)
	}

	outs := make(map[schema.GroupVersionResource][]*unstructured.Unstructured)
	for _, resList := range manifests {
		for _, res := range resList {
			r := strings.NewReader(res)
			decoder := utilyaml.NewYAMLOrJSONDecoder(r, 1024)
			utd := &unstructured.Unstructured{}
			if err := decoder.Decode(utd); err != nil {
				return nil, fmt.Errorf("decode object from resource manifest: %q error: %v", res, err)
			}
			switch gvk := utd.GetObjectKind().GroupVersionKind(); gvk {
			case kube.ServiceGVK, kube.ConfigMapGVK, kube.EnvoyFilterGVK:
				gvr := kube.ConvertToGroupVersionResource(gvk)
				outs[gvr] = append(outs[gvr], utd)
			default:
				continue
			}
		}
	}
	return outs, nil
}

func setOwnerReference(slimeboot *SlimeBoot, utd *unstructured.Unstructured) {
	// Skip if not in the same namespace
	if slimeboot.Namespace != utd.GetNamespace() {
		return
	}
	blockOwnerDeletionTrue := true
	ownerReferences := []metav1.OwnerReference{
		{
			APIVersion:         slimeboot.APIVersion,
			BlockOwnerDeletion: &blockOwnerDeletionTrue,
			Kind:               slimeboot.Kind,
			Name:               slimeboot.Name,
			UID:                slimeboot.UID,
		},
	}
	utd.SetOwnerReferences(ownerReferences)
}

func mergeObject(gvr schema.GroupVersionResource, got, utd *unstructured.Unstructured) *unstructured.Unstructured {
	ret := got.DeepCopy()
	switch gvr {
	case kube.ConfigMapGVR:
		ret.Object["data"] = utd.Object["data"]
	case kube.EnvoyFilterGVR:
		ret.Object["spec"] = utd.Object["spec"]
	case kube.ServiceGVR:
		ports, _, _ := unstructured.NestedSlice(utd.Object, "spec", "ports")
		_ = unstructured.SetNestedSlice(ret.Object, ports, "spec", "ports")
	}
	return ret
}
