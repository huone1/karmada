package clusterlocality

import (
	"context"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	workv1alpha2 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha2"
	"github.com/karmada-io/karmada/pkg/scheduler/framework"
	"github.com/karmada-io/karmada/pkg/util"
)

const (
	// Name is the name of the plugin used in the plugin registry and configurations.
	Name = "ClusterLocality"
)

// ClusterLocality is a score plugin that favors cluster that already have requested.
type ClusterLocality struct{}

var _ framework.ScorePlugin = &ClusterLocality{}

// New instantiates the clusteraffinity plugin.
func New() framework.Plugin {
	return &ClusterLocality{}
}

// Name returns the plugin name.
func (p *ClusterLocality) Name() string {
	return Name
}

// Score calculates the score on the candidate cluster.
func (p *ClusterLocality) Score(
	ctx context.Context,
	placement *policyv1alpha1.Placement,
	spec *workv1alpha2.ResourceBindingSpec,
	cluster *clusterv1alpha1.Cluster) (float64, *framework.Result) {
	if len(spec.Clusters) == 0 {
		return float64(framework.MinClusterScore), framework.NewResult(framework.Success)
	}

	replicas := util.GetSumOfReplicas(spec.Clusters)
	if replicas <= 0 {
		return float64(framework.MinClusterScore), framework.NewResult(framework.Success)
	}

	if clusterReplicas := getScheduledReplicas(cluster.Name, spec.Clusters); clusterReplicas > 0 {
		Score := float64(int64(clusterReplicas) * framework.MaxClusterScore / int64(replicas))
		return Score, framework.NewResult(framework.Success)
	}

	return float64(framework.MinClusterScore), framework.NewResult(framework.Success)
}

func getScheduledReplicas(candidate string, schedulerClusters []workv1alpha2.TargetCluster) int32 {
	for _, cluster := range schedulerClusters {
		if candidate == cluster.Name {
			return cluster.Replicas
		}
	}

	return 0
}
