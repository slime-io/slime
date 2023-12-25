package util

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	bootconfig "slime.io/slime/framework/apis/config/v1alpha1"

	log "github.com/sirupsen/logrus"
	"go.uber.org/zap/zapcore"
	ilog "istio.io/libistio/pkg/log"
	"k8s.io/klog"
)

func TimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02T15:04:05.000"))
}

func InitLog(logConfig *bootconfig.Log) error {
	// set log level
	if logConfig.LogLevel == "" {
		logConfig.LogLevel = slimeLogLevel
	}
	if logConfig.KlogLevel == 0 {
		logConfig.KlogLevel = slimeKLogLevel
	}

	scopes := make(map[string]string)
	if logConfig.IlogLevel != "" {
		parts := strings.Split(logConfig.IlogLevel, ",")
		for i := range parts {
			items := strings.Split(parts[i], ":")
			if len(items) != 2 {
				continue
			}
			scopes[items[0]] = items[1]
		}
	}

	level, err := log.ParseLevel(logConfig.LogLevel)
	if err != nil {
		return err
	}
	log.SetLevel(level)
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: time.RFC3339,
		DisableQuote:    !logConfig.EnableQuote,
	})

	var output io.Writer
	output = os.Stdout
	if logConfig.LogRotate {
		output = &lumberjack.Logger{
			Filename:   logConfig.LogRotateConfig.FilePath,
			MaxSize:    int(logConfig.LogRotateConfig.MaxSizeMB), // megabytes
			MaxBackups: int(logConfig.LogRotateConfig.MaxBackups),
			MaxAge:     int(logConfig.LogRotateConfig.MaxAgeDay), // days
			Compress:   logConfig.LogRotateConfig.Compress,       // disabled by default
		}
	}

	log.SetOutput(output)
	initKlog(logConfig.KlogLevel, output)

	SetiLog(scopes)

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
func initKlog(KlogLevel int32, output io.Writer) {
	fs = flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)

	// set log output
	if output != nil {
		fs.Set("logtostderr", "false")
		klog.SetOutput(output)
	}

	SetKlogLevel(KlogLevel)
}

// SetKlogLevel Warning: not thread safe
func SetKlogLevel(number int32) {
	fs.Set("v", fmt.Sprintf("%d", number))
}

func GetKlogLevel() string {
	return fs.Lookup("v").Value.String()
}

func SetiLog(settings map[string]string) {
	if len(settings) == 0 {
		return
	}

	stringToLevel := map[string]ilog.Level{
		"debug": ilog.DebugLevel,
		"info":  ilog.InfoLevel,
		"warn":  ilog.WarnLevel,
		"error": ilog.ErrorLevel,
		"fatal": ilog.FatalLevel,
		"none":  ilog.NoneLevel,
	}

	defaultLevel := slimeDefaultILogLevel
	if v, ok := settings[slimeDefaultScopeName]; ok {
		defaultLevel = v
	}

	for name, scope := range ilog.Scopes() {

		level := stringToLevel[defaultLevel]
		if v, ok := settings[name]; ok {
			level = stringToLevel[v]
		}
		scope.SetOutputLevel(level)
		log.Infof("set ilog scope %s:%d", name, level)
	}
}
