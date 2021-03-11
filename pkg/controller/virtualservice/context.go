/*
* @Author: yangdihang
* @Date: 2020/5/21
 */

package virtualservice

import (
	"slime.io/slime/pkg/util"
)

var HostDestinationMapping *util.SubcribeableMap

func init() {
	if HostDestinationMapping == nil {
		HostDestinationMapping = util.NewSubcribeableMap()
	}
}
