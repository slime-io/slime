package metric

import (
	"errors"
	"sync"

	data_accesslog "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"
	log "github.com/sirupsen/logrus"
)

type AccessLogConvertor struct {
	name            string                       // handler name
	cacheResult     map[string]map[string]string // meta -> value
	cacheResultCopy map[string]map[string]string
	handler         func(logEntry []*data_accesslog.HTTPAccessLogEntry) (map[string]map[string]string, error)
	convertorLock   sync.RWMutex
}

func NewAccessLogConvertor(config AccessLogConvertorConfig) *AccessLogConvertor {
	return &AccessLogConvertor{
		name:            config.Name,
		handler:         config.Handler,
		cacheResult:     make(map[string]map[string]string),
		cacheResultCopy: make(map[string]map[string]string),
	}
}

func (alc *AccessLogConvertor) Name() string {
	return alc.name
}

func (alc *AccessLogConvertor) CacheResultCopy() map[string]map[string]string {
	alc.convertorLock.RLock()
	defer alc.convertorLock.RUnlock()

	return alc.cacheResultCopy
}

func (alc *AccessLogConvertor) Convert(logEntry []*data_accesslog.HTTPAccessLogEntry) error {
	log := log.WithField("reporter", "AccessLogConvertor").WithField("function", "Convert")
	tmpResult, err := alc.handler(logEntry)
	if err != nil {
		return err
	}

	alc.convertorLock.Lock()
	defer alc.convertorLock.Unlock()

	if len(tmpResult) == 0 {
		return errors.New("tmpResult is nil")
	}

	needUpdate := false
	// merge result, copy on write
	for meta, value := range tmpResult {

		if _, ok := alc.cacheResult[meta]; !ok {
			// new meta
			log.Debugf("alc.cacheResult[%s] is not ok, inits", meta)
			needUpdate = true
			alc.cacheResult[meta] = value
			continue
		}

		// existed meta
		if updated := valueMerge(alc.cacheResult[meta], value); updated {
			needUpdate = updated
		}

	}

	if needUpdate {
		newCacheResultCopy := make(map[string]map[string]string)
		for meta, value := range alc.cacheResult {
			tmpValue := make(map[string]string)
			for k, v := range value {
				tmpValue[k] = v
			}
			newCacheResultCopy[meta] = tmpValue
		}
		alc.cacheResultCopy = newCacheResultCopy
	}

	return err
}

func valueMerge(cacheValue, tmpValue map[string]string) bool {
	needMerge := false
	for k, v := range tmpValue {
		// new key
		if _, ok := cacheValue[k]; !ok {
			needMerge = true
			cacheValue[k] = v
		}
	}
	return needMerge
}
