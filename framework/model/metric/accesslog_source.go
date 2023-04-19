package metric

import (
	"fmt"
	"net"
	"strings"

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
				log.Debugf("%s add metric from accesslog %+v", meta, result)
			}
		}

	}

	log.Debugf("successfully get metric from accesslog")
	return metric, nil
}

func (s *AccessLogSource) Reset(info string) error {
	parts := strings.Split(info, "/")
	ns, name := parts[0], parts[1]

	for _, convertor := range s.convertors {
		convertor.convertorLock.Lock()

		// reset ns/name
		for k, _ := range convertor.cacheResult {
			// it will reset all svf in ns if svc is empty
			if name == "" {
				if ns == strings.Split(k, "/")[0] {
					convertor.cacheResult[k] = map[string]string{}
				}
			} else {
				if k == info {
					convertor.cacheResult[k] = map[string]string{}
				}
			}
		}

		// sync to cacheResultCopy
		newCacheResultCopy := make(map[string]map[string]string)
		for meta, value := range convertor.cacheResult {
			tmpValue := make(map[string]string)
			for k, v := range value {
				tmpValue[k] = v
			}
			newCacheResultCopy[meta] = tmpValue
		}
		convertor.cacheResultCopy = newCacheResultCopy

		convertor.convertorLock.Unlock()
	}

	return nil
}

func (s *AccessLogSource) Fullfill(cache map[string]map[string]string) error {

	for _, convertor := range s.convertors {
		convertor.convertorLock.Lock()
		for meta, value := range cache {
			convertor.cacheResult[meta] = value
			tmpValue := make(map[string]string)
			for k, v := range value {
				tmpValue[k] = v
			}

			convertor.cacheResultCopy[meta] = tmpValue
		}
		convertor.convertorLock.Unlock()
	}
	return nil
}
