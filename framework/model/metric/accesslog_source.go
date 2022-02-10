package metric

import (
	"fmt"
	"net"

	service_accesslog "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type AccessLogSource struct {
	servePort  string
	convertors []*AccessLogConvertor
}

func NewAccessLogSource(config AccessLogSourceConfig) *AccessLogSource {
	source := &AccessLogSource{
		servePort: config.ServePort,
	}
	for _, convertorConfig := range config.AccessLogConvertorConfigs {
		source.convertors = append(source.convertors, NewAccessLogConvertor(convertorConfig))
	}
	return source
}

// StreamAccessLogs accept access log from lazy xds egress gateway
func (s *AccessLogSource) StreamAccessLogs(logServer service_accesslog.AccessLogService_StreamAccessLogsServer) error {
	log := log.WithField("reporter", "AccessLogSource").WithField("function", "StreamAccessLogs")
	for {
		message, err := logServer.Recv()
		if err != nil {
			return err
		}

		httpLogEntries := message.GetHttpLogs()
		log.Debugf("got accesslog %s", httpLogEntries.String())
		if httpLogEntries != nil {
			for _, convertor := range s.convertors {
				if err = convertor.Convert(httpLogEntries.LogEntry); err != nil {
					log.Errorf("convertor [%s] converted error: %+v", convertor.Name(), err)
				} else {
					log.Debugf("convertor %s converts successfully", convertor.Name())
				}
			}
		}
	}
}

// Start grpc server
func (s *AccessLogSource) Start() error {
	log := log.WithField("reporter", "AccessLogSource").WithField("function", "Start")
	lis, err := net.Listen("tcp", fmt.Sprintf("%s", s.servePort))
	if err != nil {
		return err
	}

	server := grpc.NewServer()
	service_accesslog.RegisterAccessLogServiceServer(server, s)

	go func() {
		log.Infof("accesslog grpc server starts on %s", s.servePort)
		if err = server.Serve(lis); err != nil {
			log.Errorf("accesslog grpc server error: %+v", err)
		}
	}()

	return nil
}

func (s *AccessLogSource) QueryMetric(queryMap QueryMap) (Metric, error) {
	log := log.WithField("reporter", "AccessLogSource").WithField("function", "QueryMetric")

	metric := make(map[string][]Result)

	for meta, handlers := range queryMap {
		if len(handlers) == 0 {
			continue
		}

		for _, handler := range handlers {
			for _, convertor := range s.convertors {
				if convertor.Name() != handler.Name {
					continue
				}
				result := Result{
					Name:  handler.Name,
					Value: convertor.CacheResultCopy()[meta],
				}
				metric[meta] = append(metric[meta], result)
				log.Debugf("add metric from accesslog %+v", result)
			}
		}

	}

	log.Debugf("successfully get metric from accesslog")
	return metric, nil
}
