package nacos

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"istio.io/pkg/log"
)

type serviceResp struct {
	Doms  []string `json:"doms"`
	Count int      `json:"count"`
}

type instance struct { // nolint: maligned
	Ip          string        `json:"ip"`
	Port        int           `json:"port"`
	Healthy     bool          `json:"healthy"`
	Valid       bool          `json:"valid"`
	Ephemeral   bool          `json:"ephemeral"`
	InstanceId  string        `json:"instanceId"`
	ClusterName string        `json:"clusterName"`
	ServiceName string        `json:"serviceName"`
	Metadata    nacosMetadata `json:"metadata,omitempty"`
}

type instanceResp struct {
	Hosts       []*instance `json:"hosts"`
	Dom         string      `json:"dom"`
	Name        string      `json:"name"`
	Env         string      `json:"env"`
	Clusters    string      `json:"clusters"`
	LastRefTime int64       `json:"lastRefTime"`
}

type nacosMetadata map[string]string

// Client for Nacos
type Client interface {
	// Instances registered on the Nacos server
	Instances() ([]*instanceResp, error)
}

type client struct {
	client  http.Client
	urls    []string
	headers map[string]string
	index   int
}

func NewClient(urls []string, headers map[string]string) Client {
	return &client{
		client:  http.Client{Timeout: 30 * time.Second},
		headers: headers,
		urls:    urls,
		index:   0,
	}
}

const (
	servicePath  = "/nacos/v1/ns/service/list?pageNo=1&pageSize=100000"
	intancesPath = "/nacos/v1/ns/instance/list?serviceName="
)

func (c *client) chooseURL() string {
	if c.index >= len(c.urls) {
		c.index = 0
	}
	url := c.urls[c.index]
	c.index++

	return url
}

func (c *client) call(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // nolint: errcheck
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code when get nacos services: %v", resp.Status)
	}
	return ioutil.ReadAll(resp.Body)
}

func (c *client) Instances() ([]*instanceResp, error) {
	url := c.chooseURL()
	log.Debug("nacos url:" + url)

	serviceData, err := c.call(url + servicePath)
	if err != nil {
		return nil, err
	}
	var services serviceResp
	if err = json.Unmarshal(serviceData, &services); err != nil {
		return nil, err
	}

	instanceAll := make([]*instanceResp, 0)
	for _, serviceName := range services.Doms {
		var instance instanceResp
		instanceData, err := c.call(url + intancesPath + "DEFAULT_GROUP@@" + serviceName)
		if err = json.Unmarshal(instanceData, &instance); err != nil {
			return nil, err
		}
		instance.Dom = serviceName
		instanceAll = append(instanceAll, &instance)
	}
	return instanceAll, nil
}
