package eureka

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/features"
	"slime.io/slime/modules/meshregistry/pkg/monitoring"
	"slime.io/slime/modules/meshregistry/pkg/source"
)

const (
	appsPath = "/apps"
)

type application struct {
	Name      string      `json:"name"`
	Instances []*instance `json:"instance"`
}

type instance struct {
	InstanceID string `json:"instanceId"`
	Hostname   string `json:"hostName"`
	IPAddress  string `json:"ipAddr"`
	Status     string `json:"status"`
	Port       port   `json:"port"`
	SecurePort port   `json:"securePort"`
	App        string `json:"app"`
	// TODO: read dataCenterInfo for AZ support
	Metadata eurekaMetadata `json:"metadata,omitempty"`
}

type port struct {
	Port    int  `json:"$"`
	Enabled bool `json:"@enabled,string"`
}

type eurekaMetadata map[string]string

// Client for Eureka
type Client interface {
	// Applications registered on the Eureka server
	Applications() ([]*application, error)
	// RegistryInfo returns the registry ID and addresses of the client
	RegistryInfo() string
}

type clients []*client

func NewClients(servers []bootstrap.EurekaServer) Client {
	clis := make(clients, 0, len(servers))
	for _, server := range servers {
		clis = append(clis, NewClient(server))
	}
	return clis
}

func (clis clients) Applications() ([]*application, error) {
	if len(clis) == 1 {
		return clis[0].Applications()
	}
	cache := make(map[string][]*instance)
	for _, cli := range clis {
		insts, err := cli.Applications()
		if err != nil {
			log.Warningf("fetch instances from server %q failed: %v", cli.urls, err)
			continue
		}
		for _, instResp := range insts {
			cache[instResp.Name] = append([]*instance(cache[instResp.Name]), instResp.Instances...)
		}
	}
	ret := make([]*application, 0, len(cache))
	for dom, hosts := range cache {
		ret = append(ret, &application{
			Name:      dom,
			Instances: hosts,
		})
	}
	return ret, nil
}

func (clis clients) RegistryInfo() string {
	info := make([]json.RawMessage, 0, len(clis))
	for _, cli := range clis {
		info = append(info, json.RawMessage(cli.RegistryInfo()))
	}
	jsonInfo, _ := json.MarshalIndent(info, "", "  ")
	return string(jsonInfo)
}

// Minimal client for Eureka server's REST APIs.
// TODO: caching
// TODO: Eureka v3 support
type client struct {
	client     http.Client
	registryID string
	urls       []string
	index      int
}

func (c *client) RegistryInfo() string {
	info := source.RegistryInfo{
		RegistryID: c.registryID,
		Addresses:  c.urls,
	}
	jsonInfo, _ := json.MarshalIndent(info, "", "  ")
	return string(jsonInfo)
}

// NewClient instantiates a new Eureka client
func NewClient(server bootstrap.EurekaServer) *client {
	return &client{
		client:     http.Client{Timeout: 30 * time.Second},
		registryID: server.RegistryID,
		urls:       server.Address,
		index:      0,
	}
}

type getApplications struct {
	Applications applications `json:"applications"`
}

type applications struct {
	Applications []*application `json:"application"`
}

func (c *client) chooseURL() string {
	if c.index >= len(c.urls) {
		c.index = 0
	}
	url := c.urls[c.index]
	c.index++

	return url
}

func (c *client) Applications() ([]*application, error) {
	url := c.chooseURL() + appsPath
	log.Debug("eureka url:" + url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	monitoring.RecordSourceClientRequest(SourceName, err == nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
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

	if c.registryID != "" {
		for _, app := range apps.Applications.Applications {
			for _, inst := range app.Instances {
				if inst.Metadata == nil {
					inst.Metadata = make(eurekaMetadata)
				}
				inst.Metadata[features.RegistryIDMetaKey] = c.registryID
			}
		}
	}

	return apps.Applications.Applications, nil
}
