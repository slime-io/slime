package util

import (
	"flag"
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"go.uber.org/zap/zapcore"
	"k8s.io/klog"
)

func TimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02T15:04:05.000"))
}

func InitLog(LogLevel string, KlogLevel int32) error {

	if LogLevel == "" {
		LogLevel = slimeLogLevel
	}
	if KlogLevel == 0 {
		KlogLevel = slimeKLogLevel
	}
	level, err := log.ParseLevel(LogLevel)
	if err != nil {
		return err
	} else {
		log.SetLevel(level)
		log.SetOutput(os.Stdout)
		log.SetFormatter(&log.TextFormatter{
			TimestampFormat: time.RFC3339,
		})
	}

	if KlogLevel != 0 {
		initKlog(KlogLevel)
	}
	return nil
}

func SetLevel(LogLevel string) error {
	level, err := log.ParseLevel(LogLevel)
	if err != nil {
		return err
	}
	log.SetLevel(level)
	return nil
}

// SetReportCaller sets whether the standard logger will include the calling
// method as a field, default false.
func SetReportCaller(support bool) {
	log.SetReportCaller(support)
}

func GetLevel() string {
	level := log.GetLevel()
	return level.String()
}

// initKlog while x<= KlogLevel in the klog.V("x").info("hello"), log will be record
func initKlog(KlogLevel int32) {
	fs = flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	SetKlogLevel(KlogLevel)
}

// SetKlogLevel Warning: not thread safe
func SetKlogLevel(number int32) {
	fs.Set("v", fmt.Sprintf("%d", number))
}

func GetKlogLevel() string {
	return fs.Lookup("v").Value.String()
}
