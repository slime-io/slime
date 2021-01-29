/*
* @Author: yangdihang
* @Date: 2020/5/21
 */

package virtualservice

import (
	"yun.netease.com/slime/pkg/util"
)

var HostDestinationMapping *util.SubcribeableMap

func init() {
	if HostDestinationMapping == nil {
		HostDestinationMapping = util.NewSubcribeableMap()
	}
}
