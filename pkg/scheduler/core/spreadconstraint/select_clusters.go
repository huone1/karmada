package spreadconstraint

import (
	"fmt"

	"k8s.io/klog/v2"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
)

// SelectBestClusters selects the cluster set based the GroupClustersInfo and placement
func SelectBestClusters(placement *policyv1alpha1.Placement, groupClustersInfo *GroupClustersInfo, needReplicas int32) ([]*clusterv1alpha1.Cluster, error) {
	if len(placement.SpreadConstraints) == 0 || isSpreadConstraintIgnored(placement) {
		var clusters []*clusterv1alpha1.Cluster
		for _, cluster := range groupClustersInfo.Clusters {
			clusters = append(clusters, cluster.Cluster)
		}
		klog.V(4).Infof("select all clusters")
		return clusters, nil
	}

	if isAvailableResourceIgnored(placement) {
		needReplicas = -1
	}

	return selectBestClustersBySpreadConstraints(placement.SpreadConstraints, groupClustersInfo, needReplicas)
}

func selectBestClustersBySpreadConstraints(spreadConstraints []policyv1alpha1.SpreadConstraint,
	groupClustersInfo *GroupClustersInfo, needReplicas int32) ([]*clusterv1alpha1.Cluster, error) {
	if len(spreadConstraints) > 1 {
		return nil, fmt.Errorf("just support single spread constraint")
	}

	spreadConstraint := spreadConstraints[0]
	if spreadConstraint.SpreadByField == policyv1alpha1.SpreadByFieldCluster {
		return selectBestClustersByCluster(spreadConstraint, groupClustersInfo, needReplicas)
	}

	return nil, fmt.Errorf("just support cluster spread constraint")
}

func selectBestClustersByCluster(spreadConstraint policyv1alpha1.SpreadConstraint, groupClustersInfo *GroupClustersInfo, needReplicas int32) ([]*clusterv1alpha1.Cluster, error) {
	totalClusterCnt := len(groupClustersInfo.Clusters)
	if spreadConstraint.MinGroups > totalClusterCnt {
		return nil, fmt.Errorf("the number of feasible clusters is less than spreadConstraint.MinGroups")
	}

	needCnt := spreadConstraint.MaxGroups
	if spreadConstraint.MaxGroups > totalClusterCnt {
		needCnt = totalClusterCnt
	}

	clusterInfos, ok := selectClustersByAvailableResource(groupClustersInfo.Clusters, int32(needCnt), needReplicas)
	if !ok {
		return nil, fmt.Errorf("no enough resource when selecting %d clusters", needCnt)
	}

	var clusters []*clusterv1alpha1.Cluster
	for i := range clusterInfos {
		clusters = append(clusters, clusterInfos[i].Cluster)
	}

	return clusters, nil
}

// if needClusterCount = 2, needReplicas = 80, member1 and member3 will be selected finally.
// because the total resource of member1 and member2 is less than needReplicas although their scores is highest
// --------------------------------------------------
// | clusterName      | member1 | member2 | member3 |
// |-------------------------------------------------
// | score            |   60    |    50   |    40   |
// |------------------------------------------------|
// |AvailableReplicas |   40    |    30   |    60   |
// |------------------------------------------------|
func selectClustersByAvailableResource(candidateClusters []*ClusterDetailInfo, needClusterCount, needReplicas int32) ([]*ClusterDetailInfo, bool) {
	retClusters := make([]*ClusterDetailInfo, needClusterCount)
	copy(retClusters, candidateClusters)
	candidateClusters = candidateClusters[needClusterCount:]

	if needReplicas == -1 {
		return retClusters, true
	}

	// the retClusters is sorted by cluster.Score descending. when the total AvailableReplicas of retClusters is less than needReplicas,
	// use the cluster with the most AvailableReplicas in candidateClusters to instead the cluster with the lowest score,
	// until checkAvailableResource returns true
	var updateID = len(retClusters) - 1
	for !checkAvailableResource(retClusters, needReplicas) && updateID >= 0 && len(candidateClusters) > 0 {
		clusterID := GetClusterWithMaxAvailableResource(candidateClusters, retClusters[updateID].AvailableReplicas)
		if clusterID == -1 {
			updateID--
			continue
		}

		rmCluster := retClusters[updateID]
		retClusters[updateID] = candidateClusters[clusterID]
		candidateClusters = append(candidateClusters[:clusterID], candidateClusters[clusterID+1:]...)
		candidateClusters = append(candidateClusters, rmCluster)
		updateID--
	}

	return retClusters, updateID >= 0
}

// GetClusterWithMaxAvailableResource returns the cluster with maxAvailableReplicas
func GetClusterWithMaxAvailableResource(candidateClusters []*ClusterDetailInfo, originReplicas int64) int {
	var maxAvailableReplicas = originReplicas
	var clusterID = -1
	for i := range candidateClusters {
		if maxAvailableReplicas < candidateClusters[i].AvailableReplicas {
			clusterID = i
			maxAvailableReplicas = candidateClusters[i].AvailableReplicas
		}
	}

	return clusterID
}

func checkAvailableResource(clusters []*ClusterDetailInfo, needReplicas int32) bool {
	var total int64

	for i := range clusters {
		total += clusters[i].AvailableReplicas
	}

	return total >= int64(needReplicas)
}

func isSpreadConstraintIgnored(placement *policyv1alpha1.Placement) bool {
	strategy := placement.ReplicaScheduling

	// If the replica division preference is 'static weighted', ignore the declaration specified by spread constraints.
	if strategy != nil && strategy.ReplicaSchedulingType == policyv1alpha1.ReplicaSchedulingTypeDivided &&
		strategy.ReplicaDivisionPreference == policyv1alpha1.ReplicaDivisionPreferenceWeighted &&
		(strategy.WeightPreference != nil && len(strategy.WeightPreference.StaticWeightList) != 0 && strategy.WeightPreference.DynamicWeight == "") {
		return true
	}

	return false
}

func isAvailableResourceIgnored(placement *policyv1alpha1.Placement) bool {
	strategy := placement.ReplicaScheduling

	// If the replica division preference is 'Duplicated', ignore the information about cluster available resource.
	if strategy == nil || strategy.ReplicaSchedulingType == policyv1alpha1.ReplicaSchedulingTypeDuplicated {
		return true
	}

	return false
}
