package nacos

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const defaultNacosTokenTTL = 5

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

	// security login
	username string
	password string
	token    *atomic.Value
	tokenTTL int64
}

func NewClient(urls []string, username, password string, headers map[string]string) Client {
	c := &client{
		client:   http.Client{Timeout: 30 * time.Second},
		headers:  headers,
		urls:     urls,
		index:    0,
		username: username,
		password: password,
		tokenTTL: defaultNacosTokenTTL, // defaulr TokenTTL as 5 second, if first login failed
		token:    &atomic.Value{},
	}
	if c.headers == nil {
		c.headers = make(map[string]string)
	}
	c.headers["Content-Type"] = "application/x-www-form-urlencoded"
	if c.username != "" && c.password != "" {
		c.login()
		c.autoRefresh()
	}
	return c
}

const (
	servicePath  = "/nacos/v1/ns/service/list?pageNo=1&pageSize=100000"
	intancesPath = "/nacos/v1/ns/instance/list?serviceName="
	loginPath    = "/nacos/v1/auth/login"
)

func (c *client) chooseURL() string {
	if c.index >= len(c.urls) {
		c.index = 0
	}
	url := c.urls[c.index]
	c.index++

	return url
}

func (c *client) call(method string, url string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, url, body)
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
		errMsg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %q when request to nacos: %s", resp.Status, string(errMsg))
	}
	return io.ReadAll(resp.Body)
}

func (c *client) Instances() ([]*instanceResp, error) {
	url := c.chooseURL()
	Scope.Debug("nacos url:" + url)

	getUrl := func(base string) string {
		token := c.token.Load()
		if token == nil {
			return base
		}
		return base + "&accessToken=" + token.(string)
	}

	serviceData, err := c.call(http.MethodGet, getUrl(url+servicePath), nil)
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
		instanceData, err := c.call(http.MethodGet, getUrl(url+intancesPath+"DEFAULT_GROUP@@"+serviceName), nil)
		if err = json.Unmarshal(instanceData, &instance); err != nil {
			return nil, err
		}
		instance.Dom = serviceName
		instanceAll = append(instanceAll, &instance)
	}
	return instanceAll, nil
}

func (c *client) login() {
	if c.username == "" || c.password == "" {
		return
	}
	needResetTTL := false
	defer func() {
		if needResetTTL {
			c.tokenTTL = defaultNacosTokenTTL
		}
	}()
	loginUrl := c.chooseURL() + loginPath
	enc := url.Values{}
	enc.Add("username", c.username)
	enc.Add("password", c.password)
	body := func() io.Reader {
		enc := url.Values{}
		enc.Add("username", c.username)
		enc.Add("password", c.password)
		return strings.NewReader(enc.Encode())
	}()
	resp, err := c.call(http.MethodPost, loginUrl, body)
	if err != nil {
		Scope.Warnf("login %s with user %s failed: %s", loginUrl, c.username, err)
		needResetTTL = true
		return
	}
	var result = struct {
		AccessToken *string `json:"accessToken,omitempty"`
		TokenTTL    *int64  `json:"tokenTtl,omitempty"`
	}{}
	err = json.Unmarshal(resp, &result)
	if err != nil {
		Scope.Warnf("parse response of login request failed: %s", err)
		needResetTTL = true
		return
	}
	if result.TokenTTL == nil {
		needResetTTL = true
	} else {
		c.tokenTTL = *result.TokenTTL
	}
	if result.AccessToken != nil {
		c.token.Store(*result.AccessToken)
	}
}

// need call login() before to init the ttl of the token
func (c *client) autoRefresh() {
	if c.username == "" || c.password == "" {
		return
	}
	go func() {
		timer := time.NewTimer(time.Duration((c.tokenTTL - c.tokenTTL/10)) * time.Second)
		for {
			<-timer.C
			c.login()
			timer.Reset(time.Duration((c.tokenTTL - c.tokenTTL/10)) * time.Second)
		}
	}()
}
