package spreadconstraint

import (
	"fmt"
	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	"sort"
)

func selectBestClustersByRegion(spreadConstraints []policyv1alpha1.SpreadConstraint,
	groupClustersInfo *GroupClustersInfo) ([]*clusterv1alpha1.Cluster, error) {
	var clusters []*clusterv1alpha1.Cluster

	spreadConstraintMap := make(map[policyv1alpha1.SpreadFieldValue]policyv1alpha1.SpreadConstraint)

	for i := range spreadConstraints {
		spreadConstraintMap[spreadConstraints[i].SpreadByField] = spreadConstraints[i]
	}

	if len(groupClustersInfo.Regions) < spreadConstraintMap[policyv1alpha1.SpreadByFieldRegion].MinGroups {
		return nil, fmt.Errorf("")
	}

	regions , ok := selectRegions(groupClustersInfo.Regions, spreadConstraintMap[policyv1alpha1.SpreadByFieldRegion], spreadConstraintMap[policyv1alpha1.SpreadByFieldCluster])
	if !ok {
		return nil, fmt.Errorf("")
	}

	return clusters, nil
}

func selectRegions(RegionInfos map[string]*RegionInfo, regionConstraint, clusterConstraint policyv1alpha1.SpreadConstraint) ([]*RegionInfo, bool) {
	var regions []*RegionInfo
	for i := range RegionInfos {
		regions = append(regions, RegionInfos[i])
	}

	sort.Slice(regions, func(i, j int) bool {
		if regions[i].Score != regions[j].Score {
			return regions[i].Score > regions[j].Score
		}

		return regions[i].Name < regions[j].Name
	})

	retRegions := regions[:regionConstraint.MinGroups]
	candidateRegions := regions[regionConstraint.MinGroups:]
	var updateID = len(retRegions) - 1
	for !CheckClusterTotalForRegion(retRegions, clusterConstraint) && updateID >= 0 && len(candidateRegions) > 0 {
		regionID := GetRegionWithMaxClusters(candidateRegions, len(retRegions[updateID].Clusters))
		if regionID == -1 {
			updateID--
			continue
		}

		rmRegion := retRegions[updateID]
		retRegions[updateID] = candidateRegions[regionID]
		candidateRegions = append(candidateRegions[:regionID], candidateRegions[regionID+1:]...)
		candidateRegions = append(candidateRegions, rmRegion)
		updateID--
	}

	return retRegions, updateID >= 0
}

func CheckClusterTotalForRegion(regions []*RegionInfo, clusterConstraint policyv1alpha1.SpreadConstraint) bool {
	var sum int
	for i := range regions {
		sum += len(regions[i].Clusters)
	}

	if sum >= clusterConstraint.MinGroups {
		return true
	}

	return false
}

func GetRegionWithMaxClusters(candidateRegions []*RegionInfo, originClusters int) int {
	var maxClusters = originClusters
	var regionID = -1
	for i := range candidateRegions {
		if maxClusters < len(candidateRegions[i].Clusters) {
			regionID = i
			maxClusters = len(candidateRegions[i].Clusters)
		}
	}

	return regionID
}
