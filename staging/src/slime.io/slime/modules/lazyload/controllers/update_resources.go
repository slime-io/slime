package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"slime.io/slime/framework/model"

	"github.com/buger/jsonparser"
	"k8s.io/apimachinery/pkg/runtime"

	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	config "slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap"
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

func addDefaultModuleValue(module *config.Config) {
	if module.Global == nil {
		module.Global = &config.Global{}
	}

	if module.Global.IstioNamespace == "" {
		module.Global.IstioNamespace = defaultIstioNs
	}
	if module.Global.SlimeNamespace == "" {
		module.Global.SlimeNamespace = defaultSlimeNs
	}
	if module.Global.Misc == nil {
		module.Global.Misc = make(map[string]string)
		module.Global.Misc["enableLeaderElection"] = "off"
	}
	if module.Global.Log == nil {
		module.Global.Log = &config.Log{
			LogLevel: "info",
		}
	}
}

func addDefaultSpecValue(spec *config.SlimeBootSpec) {
	if spec.Namespace == "" {
		spec.Namespace = defaultSlimeNs
	}
	if spec.IstioNamespace == "" {
		spec.IstioNamespace = defaultIstioNs
	}

	if spec.Component == nil {
		spec.Component = &config.Component{
			GlobalSidecar: &config.GlobalSidecar{},
		}
	}

	if spec.Component.GlobalSidecar.Port == 0 {
		spec.Component.GlobalSidecar.Port = int32(defaultPort)
	}
	if spec.Component.GlobalSidecar.ProbePort == 0 {
		spec.Component.GlobalSidecar.ProbePort = int32(defaultProbePort)
	}
	if spec.Component.GlobalSidecar.Replicas == 0 {
		spec.Component.GlobalSidecar.Replicas = int32(defaultReplicas)
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
	ctx := context.Background()
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

				// only envoyfilter is need to set istio revision label when create
				if gvr == kube.EnvoyFilterGVR && env.SelfResourceRev() != "" {
					res.SetLabels(map[string]string{model.IstioRevLabel: env.SelfResourceRev()})
				}

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

func generateValuesFormSlimeboot(wormholePort []string, env *bootstrap.Environment) (*config.SlimeBoot, map[string]interface{}, error) {
	// Deserialize to config.SlimeBoot
	specJson, slimeBoot, err := getSlimeboot(env)
	if err != nil {
		return nil, nil, fmt.Errorf("get slimeboot error: %v", err)
	}

	sort.Strings(wormholePort)
	log.Debugf("sorted wormholePort: %v", wormholePort)

	// add default value to config.SlimeBoot
	for idx, module := range slimeBoot.Spec.Module {
		if module.Kind == "lazyload" {
			addDefaultModuleValue(slimeBoot.Spec.Module[idx])
		}
	}
	addDefaultSpecValue(slimeBoot.Spec)

	// Serialize config.SlimeBoot to json
	spec, err := json.Marshal(slimeBoot.Spec)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal slimeboot spec error: %v", err)
	}

	// Insert general and general.wormholeport into general
	if len(wormholePort) > 0 {
		var pos string
		wp, err := json.Marshal(wormholePort)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal wormholePort err %s", err)
		}

		for idx := range slimeBoot.Spec.Module {
			if slimeBoot.Spec.Module[idx].Kind == "lazyload" {
				pos = fmt.Sprintf("[%d]", idx)
				break
			}
		}
		spec, err = patchSlimeboot(spec, wp, specJson, pos)
		if err != nil {
			return nil, nil, fmt.Errorf("patch slimeboot err %s", err)
		}
	}

	// Deserialize values to map[string]interface{}
	values := make(map[string]interface{})
	err = json.Unmarshal(spec, &values)
	if err != nil {
		log.Errorf("unmarshal result to values err %s", err)
		return nil, nil, err
	}
	log.Debugf("get slimeboot values %+v", values)

	return slimeBoot, values, nil
}

func patchSlimeboot(spec, wp []byte, specRaw, pos string) ([]byte, error) {
	general, _, _, err := jsonparser.Get([]byte(specRaw), "module", pos, "general")
	if err != nil {
		return nil, fmt.Errorf("get slimeboot module%s.general err %s", pos, err)
	}
	log.Debugf("get raw slimeboot module%s.general: %s", pos, general)

	// set general and general.wormholeport into general
	spec, err = jsonparser.Set(spec, general, "module", pos, "general")
	if err != nil {
		return nil, fmt.Errorf("set slimeboot general get err %s", err)
	}
	log.Debugf("set slimeboot spec.module%s.general : %s succeed", pos, general)

	spec, err = jsonparser.Set(spec, wp, "module", pos, "general", "wormholePort")
	if err != nil {
		return nil, fmt.Errorf("set slimeboot general wormholePort get err %s", err)
	}
	log.Debugf("set slimeboot spec.module%s.general.wormholePort: %s succeed", pos, wp)

	// set general.render into general
	spec, err = jsonparser.Set(spec, []byte(`"lazyload"`), "module", pos, "general", "render")
	if err != nil {
		return nil, fmt.Errorf("set slimeboot general render get err %s", err)
	}
	return spec, nil
}

func removeGsResource(m map[string]interface{}) (map[string]interface{}, error) {
	re := make(map[string]interface{})
	boot, err := json.Marshal(m)
	if err != nil {
		return re, err
	}
	boot, err = jsonparser.Set(boot, []byte(`{}`), "spec", "component", "globalSidecar", "resources")
	if err != nil {
		return re, fmt.Errorf("set resources to emtpy error %s", err.Error())
	}

	err = json.Unmarshal(boot, &re)
	if err != nil {
		return re, fmt.Errorf("unmarshal str to map[string]interface{} err %s", err.Error())
	}
	return re, nil
}

func getSlimeboot(env *bootstrap.Environment) (string, *config.SlimeBoot, error) {
	slimeBootNs := os.Getenv("WATCH_NAMESPACE")
	deployName := strings.Split(os.Getenv("POD_NAME"), "-")[0]

	utd, err := getSlimebootByOwnerRef(slimeBootNs, deployName, env)
	if err != nil {
		log.Infof("get slimeboot by ownerreferences failed with %q, try to get it by labelselector", err)
		utd, err = getSlimebootByLabelSelector(slimeBootNs, deployName, env)
		if err != nil {
			log.Infof("get slimeboot by labelselector failed with %q", err)
			return "", nil, fmt.Errorf("try to get slimeboot in namespace %s failed", slimeBootNs)
		}
	}

	// slimeboot references fields from k8s, which uses gogoprotobuf.
	res, err := removeGsResource(utd.UnstructuredContent())
	if err != nil {
		return "", nil, fmt.Errorf("remove gs resource limit error: %v", err)
	}

	// Unstructured -> SlimeBoot
	var slimeBoot config.SlimeBoot
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(res, &slimeBoot); err != nil {
		return "", nil, fmt.Errorf("convert slimeboot %s/%s to structured error: %v", slimeBootNs, utd.GetName(), err)
	}
	raw, err := json.Marshal(res["spec"])
	if err != nil {
		return "", nil, fmt.Errorf("marshal slimeboot %s/%s error: %v", slimeBootNs, utd.GetName(), err)
	}

	log.Debugf("get raw slimeboot spec: %s", string(raw))
	return string(raw), &slimeBoot, nil
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

func setOwnerReference(slimeboot *config.SlimeBoot, utd *unstructured.Unstructured) {
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
