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
