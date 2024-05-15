package eureka

import "slime.io/slime/modules/meshregistry/pkg/source/sourcetest"

var _ Client = (*MockClient)(nil)

type MockClient struct {
	sourcetest.MockPollingClient[application]
}

func (m *MockClient) Applications() ([]*application, error) {
	return m.Services()
}

func (m *MockClient) RegistryInfo() string {
	return m.MockPollingClient.RegistryInfo()
}
