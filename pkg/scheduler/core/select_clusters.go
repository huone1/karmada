package core

import (
	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	"k8s.io/klog/v2"
)

func SelectBestClusters(placement *policyv1alpha1.Placement, groupClustersInfo *GroupClustersInfo) []*clusterv1alpha1.Cluster {
	if len(placement.SpreadConstraints) != 0 {
		return selectBestClustersBySpreadConstraints(placement.SpreadConstraints, groupClustersInfo)
	}

	var clusters []*clusterv1alpha1.Cluster
	for _, cluster := range groupClustersInfo.Clusters {
		clusters = append(clusters, cluster.Cluster)
	}

	return clusters
}

func selectBestClustersBySpreadConstraints(spreadConstraints []policyv1alpha1.SpreadConstraint, groupClustersInfo *GroupClustersInfo) []*clusterv1alpha1.Cluster {
	if len(spreadConstraints) > 1 {
		klog.Errorf("just support single spread constraint")
		return nil
	}

	spreadConstraint := spreadConstraints[0]
	if spreadConstraint.SpreadByField == policyv1alpha1.SpreadByFieldCluster {
		return selectBestClustersByCluster(spreadConstraint, groupClustersInfo)
	}

	klog.Errorf("just support cluster spread constraint")
	return nil
}

func selectBestClustersByCluster(spreadConstraint policyv1alpha1.SpreadConstraint, groupClustersInfo *GroupClustersInfo) []*clusterv1alpha1.Cluster {
	TotalClusterCnt := len(groupClustersInfo.Clusters)
	if spreadConstraint.MinGroups > TotalClusterCnt {
		klog.Errorf("the number of feasible clusters is less than spreadConstraint.MinGroups")
		return nil
	}

	needCnt := spreadConstraint.MaxGroups
	if spreadConstraint.MaxGroups > TotalClusterCnt {
		needCnt = TotalClusterCnt
	}

	var clusters []*clusterv1alpha1.Cluster
	for i := 0; i < needCnt; i++ {
		clusters = append(clusters, groupClustersInfo.Clusters[i].Cluster)
	}

	return clusters
}
