package eureka

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	frameworkmodel "slime.io/slime/framework/model"
	"slime.io/slime/modules/meshregistry/model"
	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
)

var log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "eureka")

type EurekaMetadata map[string]string

type Port struct {
	Port    int  `json:"$"`
	Enabled bool `json:"@enabled,string"`
}

type Instance struct { // nolint: maligned
	InstanceID string `json:"instanceId"`
	Hostname   string `json:"hostName"`
	IPAddress  string `json:"ipAddr"`
	Status     string `json:"status"`
	Port       Port   `json:"port"`
	SecurePort Port   `json:"securePort"`
	App        string `json:"app"`
	// TODO: read dataCenterInfo for AZ support
	Metadata EurekaMetadata `json:"metadata,omitempty"`
}

func (i *Instance) GetAddress() string {
	return i.IPAddress
}

func (i *Instance) GetInstanceID() string {
	return i.InstanceID
}

func (i *Instance) GetPort() int {
	return i.Port.Port
}

func (i *Instance) IsHealthy() bool {
	return i.Status == "UP"
}

func (i *Instance) GetMetadata() map[string]string {
	return i.Metadata
}

func (i *Instance) MutableMetadata() *map[string]string {
	return (*map[string]string)(&i.Metadata)
}

func (i *Instance) Less(x *Instance) bool {
	return i.InstanceID < x.InstanceID
}

func (i *Instance) GetServiceName() string {
	return i.App
}

func (i *Instance) MutableServiceName() *string {
	return &i.App
}

type Application struct {
	Name      string      `json:"name"`
	Instances []*Instance `json:"instance"`
}

func (app *Application) GetProjectCodes() []string {
	return nil
}
func (app *Application) GetInstances() []*Instance {
	return app.Instances
}
func (app *Application) GetDomain() string {
	return app.Name
}
func (app *Application) New(name string, insts []*Instance) *Application {
	return &Application{Name: name, Instances: insts}
}

type clients []*client

func Clients(servers []bootstrap.EurekaServer) clients {
	clis := make(clients, 0, len(servers))
	for _, server := range servers {
		clis = append(clis, newClient(server.Address))
	}
	return clis
}

func (clis clients) Applications() ([]*Application, error) {
	if len(clis) == 1 {
		return clis[0].Applications()
	}
	cache := make(map[string][]*Instance)
	for _, cli := range clis {
		insts, err := cli.Applications()
		if err != nil {
			log.Warningf("fetch instances from server %v failed: %v", cli.urls, err)
			continue
		}
		for _, instResp := range insts {
			cache[instResp.Name] = append([]*Instance(cache[instResp.Name]), instResp.Instances...)
		}
	}
	ret := make([]*Application, 0, len(cache))
	for dom, hosts := range cache {
		ret = append(ret, &Application{
			Name:      dom,
			Instances: hosts,
		})
	}
	return ret, nil
}

// Minimal client for Eureka server's REST APIs.
// TODO: caching
// TODO: Eureka v3 support
type client struct {
	client http.Client
	urls   []string
	index  int
}

// newClient instantiates a new Eureka client
func newClient(urls []string) *client {
	return &client{
		client: http.Client{Timeout: 30 * time.Second},
		urls:   urls,
		index:  0,
	}
}

const (
	appsPath = "/apps"
)

type getApplications struct {
	Applications applications `json:"applications"`
}

type applications struct {
	Applications []*Application `json:"application"`
}

func (c *client) chooseURL() string {
	if c.index >= len(c.urls) {
		c.index = 0
	}
	url := c.urls[c.index]
	c.index++

	return url
}

func (c *client) Applications() ([]*Application, error) {
	url := c.chooseURL() + appsPath
	log.Debug("eureka url:" + url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // nolint: errcheck
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from EurekaSource server: %v", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apps getApplications
	if err = json.Unmarshal(data, &apps); err != nil {
		return nil, err
	}
	return apps.Applications.Applications, nil
}
