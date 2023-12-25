package mcpoverxds

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"istio.io/istio-mcp/pkg/config/schema/resource"
	mcp "istio.io/istio-mcp/pkg/mcp"
	mcpsvr "istio.io/istio-mcp/pkg/mcp/server"
	mcpxds "istio.io/istio-mcp/pkg/mcp/xds"
	mcpmodel "istio.io/istio-mcp/pkg/model"
	"istio.io/libistio/pkg/config/event"
	resource2 "istio.io/libistio/pkg/config/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	frameworkmodel "slime.io/slime/framework/model"
	"slime.io/slime/modules/meshregistry/model"
	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/monitoring"
	"slime.io/slime/modules/meshregistry/pkg/source"
)

var log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "mcpoverxds")

type McpController struct {
	ctx                context.Context
	cancel             func()
	mut                sync.Mutex
	stop, notifyPushCh chan struct{}

	mcpArgs *bootstrap.McpArgs

	lastPushVer       string
	revChangedConfigs map[mcpmodel.ConfigKey]struct{}

	mcpServer   mcp.Server
	configStore *McpConfigStore
	Handler     event.Handler
}

func NewController(args *bootstrap.RegistryArgs) (*McpController, error) {
	if args.Mcp.EnableIncPush && !args.Mcp.EnableAnnoResVer {
		return nil, errors.New("incPush enabled but anno res ver not enabled")
	}
	impl := &McpController{
		mcpArgs: args.Mcp,

		stop:              make(chan struct{}),
		notifyPushCh:      make(chan struct{}),
		configStore:       NewConfigStore(),
		revChangedConfigs: map[mcpmodel.ConfigKey]struct{}{},
	}
	// in inc-mode, we have to leave zomb to do del-event push. Thus we may have some mem which will never be freed.
	// but considering the zomb items do not have spec, it's acceptable.
	// another side effect is that the init-full-push will contain some necessary nil-spec items,
	// we'll have this in the push logic.
	impl.Handler = &XdsEventHandler{c: impl, leaveZomb: args.Mcp.EnableIncPush}
	svr, err := mcpsvr.NewServer(&mcpsvr.Options{
		XdsServerOptions: &mcpxds.ServerOptions{
			IncPush: args.Mcp.EnableIncPush,
		},
		ServerUrl: args.Mcp.ServerUrl,
	})

	if err != nil {
		return nil, fmt.Errorf("new xds server with url %s met err %v", args.Mcp.ServerUrl, err)
	} else {
		impl.mcpServer = svr
	}

	impl.ctx, impl.cancel = context.WithCancel(context.Background())
	return impl, nil
}

func (c *McpController) HandleClientsInfo(w http.ResponseWriter, r *http.Request) {
	b, err := json.MarshalIndent(c.mcpServer.ClientsInfo(), "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(w, "unable to marshal node cahce se cache: %v", err)
		return
	}
	_, _ = w.Write(b)
}

// HandleXdsCache
// query: format=yaml&layout=plain&k8sRsc=true will get all config in k8s format like kubectl get service -o yaml
// params:
//
//	res or typeUrl: ...
//	ns: namespace
//	ver: version, will return configs which is newer than that
//	format: see below constants
//	layout: see below constants
//	k8sRsc: whether to convert config to k8s resource format obj
func (c *McpController) HandleXdsCache(w http.ResponseWriter, r *http.Request) {
	const (
		LayoutGroupByGvk = "gvk"   // map[typeUrl][]config
		LayoutList       = "list"  // []config
		LayoutPlain      = "plain" // config <separator> config ... . Use `\n---\n` for yaml format

		FormatYaml = "yaml"
		FormatJson = "json"
	)

	var (
		format = FormatJson
		layout = LayoutGroupByGvk

		serializers = map[string]func(interface{}, *bytes.Buffer) error{
			FormatYaml: func(data interface{}, buf *bytes.Buffer) error {
				bs, err := yaml.Marshal(data)
				if err != nil {
					return err
				}
				if _, err = buf.Write(bs); err != nil {
					return err
				}
				return nil
			},
			FormatJson: func(data interface{}, buf *bytes.Buffer) error {
				bs, err := json.MarshalIndent(data, "", "  ")
				if err != nil {
					return err
				}
				if _, err = buf.Write(bs); err != nil {
					return err
				}
				return nil
			},
		}
	)

	values := r.URL.Query()
	ns := values.Get("ns")
	name := values.Get("name")
	res := values.Get("res")
	if res == "" {
		res = values.Get("typeUrl")
	}
	if res == "" {
		res = values.Get("gvk")
	}
	ver := values.Get("ver")
	if v := values.Get("layout"); v != "" {
		layout = v
	}
	if v := values.Get("format"); v != "" {
		format = v
	}

	ser := serializers[format]
	if ser == nil {
		http.Error(w, "unsupported format "+format, http.StatusBadRequest)
		return
	}

	gvk := resource.AllGvk
	if res != "" {
		gvk = resource.TypeUrlToGvk(res)
	}

	var k8sRsc bool
	if v := values.Get("k8sRsc"); v != "" {
		k8sRsc = true
	}

	converter := func(config mcpmodel.Config) interface{} {
		type k8sResource struct {
			metav1.TypeMeta `json:",inline"`
			Metadata        mcpmodel.Meta `json:"metadata"`
			Spec            any           `json:"spec"`
		}

		if k8sRsc {
			meta := config.Meta
			gvk := meta.GroupVersionKind
			meta.GroupVersionKind = resource.GroupVersionKind{}
			if anno := meta.Annotations; anno != nil {
				annoBak := make(map[string]string, len(anno))
				meta.Annotations = annoBak
				for k, v := range anno {
					if k == mcpmodel.AnnotationResourceVersion {
						meta.ResourceVersion = v
						continue
					}
					annoBak[k] = v
				}
			}
			apiVersion := gvk.Group
			if gvk.Version != "" {
				apiVersion += "/" + gvk.Version
			}
			return &k8sResource{
				TypeMeta: metav1.TypeMeta{
					Kind:       gvk.Kind,
					APIVersion: apiVersion,
				},
				Metadata: meta,
				Spec:     config.Spec,
			}
		}
		return config
	}
	convertConfigs := func(configs []mcpmodel.Config) []interface{} {
		items := make([]interface{}, len(configs))
		for idx, config := range configs {
			items[idx] = converter(config)
		}
		return items
	}

	var (
		configs []mcpmodel.Config
		config  *mcpmodel.Config
		err     error
	)
	if name == "" {
		configs, _, err = c.configStore.List(gvk, ns, ver)
	} else {
		config, err = c.configStore.Get(gvk, ns, name)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if config != nil {
		configs = append(configs, *config)
	}

	var (
		data interface{}
		buf  = &bytes.Buffer{}
	)

	switch layout {
	case LayoutList:
		data = convertConfigs(configs)
	case LayoutPlain:
		prevSer := ser
		ser = func(data interface{}, buffer *bytes.Buffer) error {
			listData, ok := data.([]interface{})
			if !ok {
				return fmt.Errorf("data not []interface}")
			}
			for idx, item := range listData {
				if err = prevSer(item, buf); err != nil {
					return fmt.Errorf("ser item %d %v met err %v", idx, item, err)
				}
				buf.WriteString("\n---\n")
			}
			return nil
		}
		data = convertConfigs(configs)
	case LayoutGroupByGvk:
		gvkConfigs := map[string][]interface{}{}
		for _, config := range configs {
			typeUrl := config.GroupVersionKind.String()
			gvkConfigs[typeUrl] = append(gvkConfigs[typeUrl], converter(config))
		}
		data = gvkConfigs
	default:
		http.Error(w, fmt.Sprintf("unknown layout %s", layout), http.StatusBadRequest)
		return
	}

	if data != nil {
		if err = ser(data, buf); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	_, _ = w.Write(buf.Bytes())
}

func (c *McpController) Run() {
	select {
	case <-c.ctx.Done():
		return
	default:
	}

	if c.mcpServer != nil {
		c.mcpServer.SetConfigStore(c.configStore)
	}

	go func() {
		time.Sleep(2 * time.Second)
		c.start(c.ctx)
	}()
}

func (c *McpController) push() {
	c.mut.Lock()
	defer c.mut.Unlock()

	var req *mcp.PushRequest
	if len(c.revChangedConfigs) > 0 {
		req = &mcp.PushRequest{RevChangeConfigs: c.revChangedConfigs}
		c.revChangedConfigs = map[mcpmodel.ConfigKey]struct{}{}
	}

	version := c.configStore.Version(resource.AllNamespace)
	if req == nil && version == c.lastPushVer {
		return
	}

	log.Infof("new push for version: %s", version)
	c.mcpServer.NotifyPush(req)
	c.lastPushVer = version
	monitoring.RecordMcpPush()
}

func (c *McpController) start(ctx context.Context) {
	go c.mcpServer.Start(ctx)

	if c.mcpArgs.EnableIncPush && c.mcpArgs.CleanZombieInterval > 0 && c.mcpServer.ClientsInfo() != nil {
		// if mcp server supports ClientsInfo, it at least returns an empty data rather than nil.
		go func() {
			ticker := time.NewTicker(time.Duration(c.mcpArgs.CleanZombieInterval))
			defer ticker.Stop()
			// clean zombie
			for {
				select {
				case <-c.stop:
					return
				case <-ticker.C:
					gvkVers := c.configStore.dumpGvkResourceVersions() // gvk1-v1, gvk2-v2, gvk3-v3
					clientInfos := c.mcpServer.ClientsInfo()
					clientGvkMinVers := make(map[resource.GroupVersionKind]string, len(gvkVers))
					for _, ci := range clientInfos {
						for gvk, ps := range ci.PushStatus {
							exist, ok := clientGvkMinVers[gvk]
							if !ok || ps.AckVer < exist {
								clientGvkMinVers[gvk] = ps.AckVer
							}
						}
					}
					// gvk1-v01, gvk2-v02
					// we think later-connected clients don't care gvk3 zombie later than v3

					for gvk := range gvkVers {
						if minVer, ok := clientGvkMinVers[gvk]; ok {
							gvkVers[gvk] = minVer
						}
					}

					cnt := c.configStore.ClearZombie(gvkVers)
					if len(cnt) > 0 {
						log.Infof("have cleaned zombie configs older than %+v %+v", gvkVers, cnt)
					}
				}
			}
		}()
	}

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-c.stop:
				return
			case <-ticker.C:
				c.push()
			case <-c.notifyPushCh:
				c.push()
			}
		}
	}()
}

func (c *McpController) convert(e event.Event) (*mcpmodel.Config, string, string, error) {
	if e.Resource == nil || e.Source == nil {
		return nil, "", "", fmt.Errorf("ev resource or source or source.resource nil")
	}

	var (
		gvk = resource.TypeUrlToGvk(e.Source.GroupVersionKind().String())
		ver resource2.Version
	)
	if !source.IsInternalResource(gvk) {
		k8sVer, err := strconv.ParseUint(string(ver), 10, 64)
		if err != nil {
			return nil, "", "", fmt.Errorf("k8s resource event %v version %v not uint", e.Resource.Metadata, ver)
		}
		ver = resource2.Version(fmt.Sprintf("%032d", k8sVer))
	}

	meta := mcpmodel.Meta{
		GroupVersionKind:  gvk,
		CreationTimestamp: e.Resource.Metadata.CreateTime,
		Labels:            e.Resource.Metadata.Labels,
		Annotations:       e.Resource.Metadata.Annotations,
		Name:              e.Resource.Metadata.FullName.Name.String(),
		Namespace:         e.Resource.Metadata.FullName.Namespace.String(),
		ResourceVersion:   string(ver),
	}

	conf := &mcpmodel.Config{
		Meta: meta,
		Spec: e.Resource.Message,
	}

	if c.mcpArgs.EnableAnnoResVer && meta.ResourceVersion != "" {
		mcpmodel.UpdateAnnotationResourceVersion(conf)
	}

	return conf, meta.Namespace, meta.Name, nil
}

func (c *McpController) NotifyPush() bool {
	select {
	case c.notifyPushCh <- struct{}{}:
		return true
	default:
	}
	return false
}

func (c *McpController) RecordConfigRevChange(gvk resource.GroupVersionKind, nn mcpmodel.NamespacedName) {
	c.mut.Lock()
	defer c.mut.Unlock()

	c.revChangedConfigs[mcpmodel.ConfigKey{
		Kind:      gvk,
		Name:      nn.Name,
		Namespace: nn.Namespace,
	}] = struct{}{}
}

type XdsEventHandler struct {
	leaveZomb bool
	c         *McpController
}

func (h *XdsEventHandler) Handle(e event.Event) {
	if e.Kind == event.None || e.Kind == event.FullSync || e.Kind == event.Reset {
		return
	}
	config, ns, name, err := h.c.convert(e)
	if err != nil {
		log.Errorf("convert event %v met err %v", e, err)
		return
	}

	var revChanged bool
	switch e.Kind {
	case event.Added, event.Updated:
		revChanged = h.c.configStore.Update(ns, config.GroupVersionKind, name, config)
	case event.Deleted:
		gvk := config.GroupVersionKind
		if h.leaveZomb {
			config.Spec = nil
		} else {
			config = nil
		}
		revChanged = h.c.configStore.Update(ns, gvk, name, config)
	default:
		// nothing
	}

	if revChanged {
		h.c.RecordConfigRevChange(config.GroupVersionKind, mcpmodel.NamespacedName{
			Name:      name,
			Namespace: ns,
		})
	}
	h.c.NotifyPush()
}
