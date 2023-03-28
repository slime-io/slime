package nacos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
)

const defaultNacosTokenTTL = 5

var serviceListPageSize = 1000

type nacosNamespace struct {
	Namespace         string `json:"namespace,omitempty"`
	NamespaceShowName string `json:"namespaceShowName,omitempty"`
}

type namespaceResp struct {
	Data []*nacosNamespace `json:"data,omitempty"`
}

type serviceResp struct {
	Doms  []string `json:"doms,omitempty"`
	Count int      `json:"count,omitempty"`
}

type catalogServiceInfo struct {
	Name      string `json:"name,omitempty"`
	GroupName string `json:"groupName,omitempty"`
}

type catalogServiceListResp struct {
	ServiceList []*catalogServiceInfo `json:"serviceList,omitempty"`
	Count       int                   `json:"count,omitempty"`
}

type nacosMetadata map[string]string

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
	Hosts []*instance `json:"hosts"`
	Dom   string      `json:"dom"`
}

// Client for Nacos
type Client interface {
	// Instances registered on the Nacos server
	Instances() ([]*instanceResp, error)
}

type clients []*client

func NewClients(
	servers []bootstrap.NacosServer,
	metaKeyNamespace, metaKeyGroup string,
	headers map[string]string) Client {
	clis := make(clients, 0, len(servers))
	for _, server := range servers {
		clis = append(clis, newClient(server, metaKeyNamespace, metaKeyGroup, headers))
	}
	return clis
}

func (clis clients) Instances() ([]*instanceResp, error) {
	if len(clis) == 1 {
		return clis[0].Instances()
	}
	cache := make(map[string][]*instance)
	for _, cli := range clis {
		insts, err := cli.Instances()
		if err != nil {
			log.Warning("fetch instances from server failed: %v", cli.urls, err)
			continue
		}
		for _, instResp := range insts {
			cache[instResp.Dom] = append([]*instance(cache[instResp.Dom]), instResp.Hosts...)
		}
	}
	ret := make([]*instanceResp, 0, len(cache))
	for dom, hosts := range cache {
		ret = append(ret, &instanceResp{
			Dom:   dom,
			Hosts: hosts,
		})
	}
	return ret, nil
}

type client struct {
	client  http.Client
	urls    []string
	headers map[string]string

	// fetch instances from a specific namespace and group, or from all of the namespaces.
	namespaceId, group             string // if not set which means public and DEFAULT_GROUP
	metaKeyNamespace, metaKeyGroup string
	fetchAllNamespaces             bool
	injectNsGroupIntoMeta          bool

	// security login
	username string
	password string
	token    *atomic.Value
	tokenTTL int64
}

func newClient(
	server bootstrap.NacosServer,
	metaKeyNamespace, metaKeyGroup string,
	headers map[string]string) *client {
	c := &client{
		client:             http.Client{Timeout: 30 * time.Second},
		headers:            headers,
		urls:               server.Address,
		namespaceId:        server.Namespace,
		group:              server.Group,
		metaKeyNamespace:   metaKeyNamespace,
		metaKeyGroup:       metaKeyGroup,
		fetchAllNamespaces: server.AllNamespaces,
		username:           server.Username,
		password:           server.Password,
		tokenTTL:           defaultNacosTokenTTL, // default TokenTTL as 5 second, if first login failed
		token:              &atomic.Value{},
	}
	c.injectNsGroupIntoMeta = c.metaKeyNamespace != "" || c.metaKeyGroup != ""
	if c.headers == nil {
		c.headers = make(map[string]string)
	}
	c.headers["Content-Type"] = "application/x-www-form-urlencoded"
	c.headers["Accept"] = "application/json"
	if c.username != "" && c.password != "" {
		c.login()
		c.autoRefresh()
	}
	return c
}

const (
	namespaceListAPI      = "/nacos/v1/console/namespaces"
	serviceListAPI        = "/nacos/v1/ns/service/list"
	catalogServiceListAPI = "/nacos/v1/ns/catalog/services"
	intancesListAPI       = "/nacos/v1/ns/instance/list"
	loginAPI              = "/nacos/v1/auth/login"
)

func encodeQuery(param map[string]string) string {
	if len(param) == 0 {
		return ""
	}
	enc := url.Values{}
	for k, v := range param {
		enc.Add(k, v)
	}
	return enc.Encode()
}

func (c *client) call(api string, method string, header map[string]string, queryParam map[string]string, body io.Reader) ([]byte, error) {
	query := encodeQuery(queryParam)
	appendUrl := func(url string) string {
		if query == "" {
			return url
		}
		return url + "?" + query
	}
	var lastErr error
	var bodyContent []byte
	if body != nil {
		r, err := io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("call nacos api %s with body but read failed: %s", api, err)
		}
		bodyContent = r
	}
	l := len(c.urls)
	shift := rand.Intn(l)
	for i := 0; i < l; i++ {
		var curBody io.Reader
		if len(bodyContent) != 0 {
			curBody = bytes.NewReader(bodyContent)
		}
		url := c.urls[(i+shift)%l] + api
		resp, err := c.doCall(appendUrl(url), method, header, curBody)
		if err == nil {
			return resp, nil
		}
		log.Debugf("call nacos api %s failed: %s", url, err)
		lastErr = err
	}
	return nil, lastErr
}

func (c *client) doCall(url string, method string, header map[string]string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
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
	var fetcher func() (map[string][]*instance, error)
	if c.fetchAllNamespaces {
		fetcher = c.allNamespacesInstances
	} else {
		fetcher = func() (map[string][]*instance, error) {
			return c.namespacedGroupedInstances(c.namespaceId, c.group)
		}
	}
	m, err := fetcher()
	if err != nil {
		log.Errorf("do get instances failed: %s", err)
		return nil, err
	}
	resp := make([]*instanceResp, 0, len(m))
	for svc, instances := range m {
		resp = append(resp, &instanceResp{
			Dom:   svc,
			Hosts: instances,
		})
	}
	return resp, nil
}

func (c *client) pagingListServices(namespaceId, groupName string, pageNo int) (*serviceResp, error) {
	var sr serviceResp
	param := map[string]string{
		"namespaceId": namespaceId,
		"groupName":   groupName,
		"pageSize":    fmt.Sprintf("%d", serviceListPageSize),
		"pageNo":      fmt.Sprintf("%d", pageNo),
	}
	c.injectAuthParam(param)
	resp, err := c.call(serviceListAPI, http.MethodGet, c.headers, param, nil)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(resp, &sr); err != nil {
		return nil, err
	}
	return &sr, nil
}

func (c *client) listServices(namespaceId, groupName string) ([]string, error) {
	probeServiceResp, err := c.pagingListServices(namespaceId, groupName, 1)
	if err != nil {
		return nil, err
	}
	doms := probeServiceResp.Doms
	if probeServiceResp.Count > serviceListPageSize {
		pageCount := probeServiceResp.Count/serviceListPageSize + 1
		for page := 2; page <= pageCount; page++ {
			sr, err := c.pagingListServices(namespaceId, groupName, page)
			if err != nil {
				return nil, err
			}
			doms = append(doms, sr.Doms...)
		}
	}
	return doms, nil
}

func (c *client) pagingListCatalogServices(namespaceId string, pageNo int) (*catalogServiceListResp, error) {
	var csr catalogServiceListResp
	param := map[string]string{
		"namespaceId":  namespaceId,
		"haseIpCount":  "false",
		"withIntances": "false",
		"pageSize":     fmt.Sprintf("%d", serviceListPageSize),
		"pageNo":       fmt.Sprintf("%d", pageNo),
	}
	c.injectAuthParam(param)
	resp, err := c.call(catalogServiceListAPI, http.MethodGet, c.headers, param, nil)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(resp, &csr); err != nil {
		return nil, err
	}
	return &csr, nil
}

func (c *client) listCatalogServices(namespaceId string) ([]*catalogServiceInfo, error) {
	probeServiceResp, err := c.pagingListCatalogServices(namespaceId, 1)
	if err != nil {
		return nil, err
	}
	groupedServices := probeServiceResp.ServiceList
	if probeServiceResp.Count > serviceListPageSize {
		pageCount := probeServiceResp.Count/serviceListPageSize + 1
		for page := 2; page <= pageCount; page++ {
			csr, err := c.pagingListCatalogServices(namespaceId, page)
			if err != nil {
				return nil, err
			}
			groupedServices = append(groupedServices, csr.ServiceList...)
		}
	}
	return groupedServices, nil
}

func (c *client) listInstances(namespaceId, groupName, serviceName string) ([]*instance, error) {
	var ir instanceResp
	param := map[string]string{
		"namespaceId": namespaceId,
		"groupName":   groupName,
		"serviceName": serviceName,
	}
	c.injectAuthParam(param)
	resp, err := c.call(intancesListAPI, http.MethodGet, c.headers, param, nil)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(resp, &ir); err != nil {
		return nil, err
	}
	if c.injectNsGroupIntoMeta {
		for _, inst := range ir.Hosts {
			if inst.Metadata == nil {
				inst.Metadata = make(nacosMetadata)
			}
			// replaces the original value
			if c.metaKeyNamespace != "" {
				inst.Metadata[c.metaKeyNamespace] = namespaceId
			}
			if c.metaKeyGroup != "" {
				inst.Metadata[c.metaKeyGroup] = groupName
			}
		}
	}
	return ir.Hosts, nil
}

func (c *client) listNamespaces() ([]*nacosNamespace, error) {
	var nr namespaceResp
	param := map[string]string{}
	c.injectAuthParam(param)
	resp, err := c.call(namespaceListAPI, http.MethodGet, c.headers, param, nil)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(resp, &nr); err != nil {
		return nil, err
	}
	return nr.Data, nil

}

func (c *client) namespacedGroupedInstances(namespaceId, groupName string) (map[string][]*instance, error) {
	svcs, err := c.listServices(namespaceId, groupName)
	if err != nil {
		log.Errorf("list services in namespace %q group %q failed: %s", namespaceId, groupName, err)
		return nil, err
	}
	svcInstances := make(map[string][]*instance, len(svcs))
	for _, svc := range svcs {
		instances, err := c.listInstances(namespaceId, groupName, svc)
		if err != nil {
			log.Warnf("list instances of service %q in namespace %q group %q failed: %s", namespaceId, groupName, svc, err)
			// try best
			continue
		}
		svcInstances[svc] = instances
	}
	return svcInstances, nil
}

func (c *client) namespacedInstances(namespaceId string) (map[string][]*instance, error) {
	svcs, err := c.listCatalogServices(namespaceId)
	if err != nil {
		log.Errorf("list services using catalog api in namespace %q failed: %s", namespaceId, err)
		return nil, err
	}
	svcInstances := make(map[string][]*instance, len(svcs))
	for _, svc := range svcs {
		instances, err := c.listInstances(namespaceId, svc.GroupName, svc.Name)
		if err != nil {
			log.Warnf("list instances of service %q in namespace %q group %q failed: %s", namespaceId, svc.GroupName, svc.Name, err)
			// try best
			continue
		}

		svcInstances[svc.Name] = append(svcInstances[svc.Name], instances...)
	}
	return svcInstances, nil
}

func (c *client) allNamespacesInstances() (map[string][]*instance, error) {
	nsList, err := c.listNamespaces()
	if err != nil {
		log.Errorf("list namespaces failed: %s", err)
		return nil, err
	}
	svcInstances := make(map[string][]*instance)
	for _, ns := range nsList {
		instances, err := c.namespacedInstances(ns.Namespace)
		if err != nil {
			log.Warnf("get all instances in namespace %q failed: %s", ns.Namespace, err)
			// try best
			continue
		}
		for k, v := range instances {
			svcInstances[k] = append(svcInstances[k], v...)
		}
	}
	return svcInstances, nil
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
	body := func() io.Reader {
		enc := url.Values{}
		enc.Add("username", c.username)
		enc.Add("password", c.password)
		return strings.NewReader(enc.Encode())
	}()
	resp, err := c.call(loginAPI, http.MethodPost, c.headers, nil, body)
	if err != nil {
		log.Warnf("login with user %s failed: %s", c.username, err)
		needResetTTL = true
		return
	}
	var result = struct {
		AccessToken *string `json:"accessToken,omitempty"`
		TokenTTL    *int64  `json:"tokenTtl,omitempty"`
	}{}
	err = json.Unmarshal(resp, &result)
	if err != nil {
		log.Warnf("parse response of login request failed: %s", err)
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

func (c *client) injectAuthParam(param map[string]string) {
	v := c.token.Load()
	token, ok := v.(string)
	if ok {
		param["accessToken"] = token
	}
}
