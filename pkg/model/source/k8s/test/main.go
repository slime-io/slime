package main

import (
	"context"
	"fmt"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"time"
)

const query = `sum(container_cpu_usage_seconds_total{pod="productpage-v1-64794f5db4-2pgl4",image=""})by(pod)`
func main() {
	client, err := api.NewClient(api.Config{
		Address: "http://localhost:9090",
	})
	if err == nil {
		api := v1.NewAPI(client)
		v,w,e := api.Query(context.Background(),query,time.Now())
		fmt.Printf("%s,%s,%s",v,w,e)
	}
}
