/*
* @Author: yangdihang
* @Date: 2021/1/25
 */

package controllers

import "slime.io/slime/framework/util"

// HostSubsetMapping You can query which subsets the Host contains, which is defined in the DestinationRule
var HostSubsetMapping *util.SubcribeableMap

// HostDestinationMapping You can query the back-end service of the host, which is defined in the VirtualService
var HostDestinationMapping *util.SubcribeableMap

func init() {
	if HostSubsetMapping == nil {
		HostSubsetMapping = util.NewSubcribeableMap()
	}
	if HostDestinationMapping == nil {
		HostDestinationMapping = util.NewSubcribeableMap()
	}
}
