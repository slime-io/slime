package eureka

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"istio.io/pkg/log"
)

type application struct {
	Name      string      `json:"name"`
	Instances []*instance `json:"instance"`
}

type instance struct { // nolint: maligned
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
}

// Minimal client for Eureka server's REST APIs.
// TODO: caching
// TODO: Eureka v3 support
type client struct {
	client http.Client
	urls   []string
	index  int
}

// NewClient instantiates a new Eureka client
func NewClient(urls []string) Client {
	return &client{
		client: http.Client{Timeout: 30 * time.Second},
		urls:   urls,
		index:  0,
	}
}

const statusUp = "UP"

const (
	appsPath = "/apps"
)

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
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // nolint: errcheck
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from EurekaSource server: %v", resp.Status)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apps getApplications
	if err = json.Unmarshal(data, &apps); err != nil {
		return nil, err
	}

	return apps.Applications.Applications, nil
}
