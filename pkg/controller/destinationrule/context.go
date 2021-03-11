/*
* @Author: yangdihang
* @Date: 2021/1/25
 */

package destinationrule

import "slime.io/slime/pkg/util"

var HostSubsetMapping *util.SubcribeableMap

func init() {
	if HostSubsetMapping == nil {
		HostSubsetMapping = util.NewSubcribeableMap()
	}
}
