package metric

type MockSource struct{}

func NewMockSource() *MockSource {
	return &MockSource{}
}

func (s *MockSource) Start() error {
	return nil
}

func (s *MockSource) QueryMetric(queryMap QueryMap) (Metric, error) {
	metric := make(map[string][]Result)
	for metaInfo := range queryMap {
		metric[metaInfo] = []Result{}
	}
	return metric, nil
}

func (s *MockSource) Reset(info string) error {
	return nil
}

func (s *MockSource) Fullfill(cache map[string]map[string]string) error {
	return nil
}
