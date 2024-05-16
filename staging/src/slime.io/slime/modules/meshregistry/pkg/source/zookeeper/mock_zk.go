package zookeeper

import (
	"github.com/go-zookeeper/zk"

	"slime.io/slime/modules/meshregistry/pkg/source/sourcetest"
)

var _ ZkConn = (*MockZkConn)(nil)

type MockZkConn struct {
	sourcetest.MockZookeeperClient
}

func (m *MockZkConn) Store(*zk.Conn) {}

func (m *MockZkConn) Load() any {
	return m.MockZookeeperClient
}

func (m *MockZkConn) Children(path string) ([]string, error) {
	return m.MockZookeeperClient.Children(path)
}

func (m *MockZkConn) ChildrenW(path string) ([]string, <-chan zk.Event, error) {
	return m.MockZookeeperClient.ChildrenW(path)
}

func (m *MockZkConn) LoadData(path string) error {
	return m.MockZookeeperClient.Load(path)
}
