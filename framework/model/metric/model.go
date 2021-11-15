package metric

type QueryMap map[string][]Handler

type Handler struct {
	Name  string
	Query string
}

type Metric map[string][]Result

type Result struct {
	Name  string
	Value map[string]string
}
