package nacos

import "slime.io/slime/modules/meshregistry/pkg/source/sourcetest"

var _ Client = (*MockClient)(nil)

type MockClient struct {
	sourcetest.MockPollingClient[instanceResp]
}

func (m *MockClient) Instances() ([]*instanceResp, error) {
	return m.Services()
}

func (m *MockClient) RegistryInfo() string {
	return m.MockPollingClient.RegistryInfo()
}
