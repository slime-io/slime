/*
* @Author: yangdihang
* @Date: 2020/8/31
 */

package util

import (
	"os"
	"os/signal"
	"syscall"
)

// WaitSignal awaits for SIGINT or SIGTERM and closes the channel
func WaitSignal(stop chan struct{}) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	close(stop)
}
