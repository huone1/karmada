package spreadconstraint

import (
	"fmt"
	"sort"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
)

func selectBestClustersByRegion(spreadConstraints []policyv1alpha1.SpreadConstraint,
	groupClustersInfo *GroupClustersInfo) ([]*clusterv1alpha1.Cluster, error) {
	var clusters []*clusterv1alpha1.Cluster
	var candidateClusters []*ClusterDetailInfo
	spreadConstraintMap := make(map[policyv1alpha1.SpreadFieldValue]policyv1alpha1.SpreadConstraint)

	for i := range spreadConstraints {
		spreadConstraintMap[spreadConstraints[i].SpreadByField] = spreadConstraints[i]
	}

	if len(groupClustersInfo.Regions) < spreadConstraintMap[policyv1alpha1.SpreadByFieldRegion].MinGroups {
		return nil, fmt.Errorf("the number of feasible region is less than spreadConstraint.MinGroups")
	}

	// firstly, select regions which have enough clusters to satisfy the cluster and region propagation constraints
	regions, ok := selectRegions(groupClustersInfo.Regions, spreadConstraintMap[policyv1alpha1.SpreadByFieldRegion], spreadConstraintMap[policyv1alpha1.SpreadByFieldCluster])
	if !ok {
		return nil, fmt.Errorf("the number of clusters is less than the cluster spreadConstraint.MinGroups")
	}

	// secondly, select the clusters with the highest score in per region,
	for i := range regions {
		clusters = append(clusters, regions[i].Clusters[0].Cluster)
		candidateClusters = append(candidateClusters, regions[i].Clusters[1:]...)
	}

	needCnt := len(candidateClusters) + len(clusters)
	if needCnt > spreadConstraintMap[policyv1alpha1.SpreadByFieldCluster].MaxGroups {
		needCnt = spreadConstraintMap[policyv1alpha1.SpreadByFieldCluster].MaxGroups
	}

	// thirdly, select the remaining Clusters based cluster.Score
	sortClusters(candidateClusters)

	j := 0
	for i := len(clusters); i < needCnt; i++ {
		clusters = append(clusters, candidateClusters[j].Cluster)
		j++
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
	var replaceID = len(retRegions) - 1
	for !CheckClusterTotalForRegion(retRegions, clusterConstraint) && replaceID >= 0 && len(candidateRegions) > 0 {
		regionID := GetRegionWithMaxClusters(candidateRegions, len(retRegions[replaceID].Clusters))
		if regionID == -1 {
			replaceID--
			continue
		}

		rmRegion := retRegions[replaceID]
		retRegions[replaceID] = candidateRegions[regionID]
		candidateRegions = append(candidateRegions[:regionID], candidateRegions[regionID+1:]...)
		candidateRegions = append(candidateRegions, rmRegion)
		replaceID--
	}

	return retRegions, replaceID >= 0
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
