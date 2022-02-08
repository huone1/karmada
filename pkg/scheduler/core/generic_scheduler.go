package core

import (
	"context"
	"fmt"
	"sort"
	"time"

	"k8s.io/klog/v2"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	workv1alpha2 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha2"
	"github.com/karmada-io/karmada/pkg/scheduler/cache"
	"github.com/karmada-io/karmada/pkg/scheduler/framework"
	"github.com/karmada-io/karmada/pkg/scheduler/framework/runtime"
	"github.com/karmada-io/karmada/pkg/scheduler/metrics"
	"github.com/karmada-io/karmada/pkg/util"
)

// ScheduleAlgorithm is the interface that should be implemented to schedule a resource to the target clusters.
type ScheduleAlgorithm interface {
	Schedule(context.Context, *policyv1alpha1.Placement, *workv1alpha2.ResourceBindingSpec) (scheduleResult ScheduleResult, err error)
}

// ScheduleResult includes the clusters selected.
type ScheduleResult struct {
	SuggestedClusters []workv1alpha2.TargetCluster
}

type genericScheduler struct {
	schedulerCache    cache.Cache
	scheduleFramework framework.Framework
}

// NewGenericScheduler creates a genericScheduler object.
func NewGenericScheduler(
	schedCache cache.Cache,
	plugins []string,
) ScheduleAlgorithm {
	return &genericScheduler{
		schedulerCache:    schedCache,
		scheduleFramework: runtime.NewFramework(plugins),
	}
}

func (g *genericScheduler) Schedule(ctx context.Context, placement *policyv1alpha1.Placement, spec *workv1alpha2.ResourceBindingSpec) (result ScheduleResult, err error) {
	clusterInfoSnapshot := g.schedulerCache.Snapshot()
	if clusterInfoSnapshot.NumOfClusters() == 0 {
		return result, fmt.Errorf("no clusters available to schedule")
	}

	feasibleClusters, err := g.findClustersThatFit(ctx, g.scheduleFramework, placement, &spec.Resource, clusterInfoSnapshot)
	if err != nil {
		return result, fmt.Errorf("failed to findClustersThatFit: %v", err)
	}
	if len(feasibleClusters) == 0 {
		return result, fmt.Errorf("no clusters fit")
	}
	klog.V(4).Infof("feasible clusters found: %v", feasibleClusters)

	clustersScore, err := g.prioritizeClusters(ctx, g.scheduleFramework, placement, spec, feasibleClusters)
	if err != nil {
		return result, fmt.Errorf("failed to prioritizeClusters: %v", err)
	}
	klog.V(4).Infof("feasible clusters scores: %v", clustersScore)

	clusters := g.selectClusters(clustersScore, placement.SpreadConstraints)

	clustersWithReplicas, err := g.assignReplicas(clusters, placement.ReplicaScheduling, spec)
	if err != nil {
		return result, fmt.Errorf("failed to assignReplicas: %v", err)
	}
	result.SuggestedClusters = clustersWithReplicas

	return result, nil
}

// findClustersThatFit finds the clusters that are fit for the placement based on running the filter plugins.
func (g *genericScheduler) findClustersThatFit(
	ctx context.Context,
	fwk framework.Framework,
	placement *policyv1alpha1.Placement,
	resource *workv1alpha2.ObjectReference,
	clusterInfo *cache.Snapshot) ([]*clusterv1alpha1.Cluster, error) {
	defer metrics.ScheduleStep(metrics.ScheduleStepFilter, time.Now())

	var out []*clusterv1alpha1.Cluster
	clusters := clusterInfo.GetReadyClusters()
	for _, c := range clusters {
		resMap := fwk.RunFilterPlugins(ctx, placement, resource, c.Cluster())
		res := resMap.Merge()
		if !res.IsSuccess() {
			klog.V(4).Infof("cluster %q is not fit", c.Cluster().Name)
		} else {
			out = append(out, c.Cluster())
		}
	}

	return out, nil
}

// prioritizeClusters prioritize the clusters by running the score plugins.
func (g *genericScheduler) prioritizeClusters(
	ctx context.Context,
	fwk framework.Framework,
	placement *policyv1alpha1.Placement,
	spec *workv1alpha2.ResourceBindingSpec,
	clusters []*clusterv1alpha1.Cluster) (result framework.ClusterScoreList, err error) {
	defer metrics.ScheduleStep(metrics.ScheduleStepScore, time.Now())

	scoresMap, err := fwk.RunScorePlugins(ctx, placement, spec, clusters)
	if err != nil {
		return result, err
	}

	if klog.V(5).Enabled() {
		for plugin, clusterScoreList := range scoresMap {
			klog.Infof("Plugin %s scores on %v %v/%v => %v",
				plugin, spec.Resource.Namespace, spec.Resource.Kind, spec.Resource.Name, clusterScoreList)
		}
	}

	result = make(framework.ClusterScoreList, len(clusters))
	for i := range clusters {
		result[i] = framework.ClusterScore{Cluster: clusters[i], Score: 0}
		for j := range scoresMap {
			result[i].Score += scoresMap[j][i].Score
		}
	}

	return result, nil
}

func (g *genericScheduler) selectClusters(clustersScoreList framework.ClusterScoreList, spreadConstraints []policyv1alpha1.SpreadConstraint) []*clusterv1alpha1.Cluster {
	defer metrics.ScheduleStep(metrics.ScheduleStepSelect, time.Now())

	if len(spreadConstraints) != 0 {
		return g.matchSpreadConstraints(clustersScoreList, spreadConstraints)
	}

	var feasibleClusters []*clusterv1alpha1.Cluster

	sort.Sort(clustersScoreList)
	for _, clustersScore := range clustersScoreList {
		feasibleClusters = append(feasibleClusters, clustersScore.Cluster)
	}
	return feasibleClusters
}

func (g *genericScheduler) matchSpreadConstraints(clusters framework.ClusterScoreList, spreadConstraints []policyv1alpha1.SpreadConstraint) []*clusterv1alpha1.Cluster {
	state := util.NewSpreadGroup()
	g.runSpreadConstraintsFilter(clusters, spreadConstraints, state)
	return g.calSpreadResult(state)
}

// Now support spread by cluster. More rules will be implemented later.
func (g *genericScheduler) runSpreadConstraintsFilter(clusters framework.ClusterScoreList, spreadConstraints []policyv1alpha1.SpreadConstraint, spreadGroup *util.SpreadGroup) {
	for _, spreadConstraint := range spreadConstraints {
		spreadGroup.InitialGroupRecord(spreadConstraint)
		if spreadConstraint.SpreadByField == policyv1alpha1.SpreadByFieldCluster {
			g.groupByFieldCluster(clusters, spreadConstraint, spreadGroup)
		}
	}
}

func (g *genericScheduler) groupByFieldCluster(clusters framework.ClusterScoreList, spreadConstraint policyv1alpha1.SpreadConstraint, spreadGroup *util.SpreadGroup) {
	for _, cluster := range clusters {
		clusterGroup := cluster.Cluster.Name
		spreadGroup.GroupRecord[spreadConstraint][clusterGroup] = append(spreadGroup.GroupRecord[spreadConstraint][clusterGroup], cluster)
	}
}

func (g *genericScheduler) calSpreadResult(spreadGroup *util.SpreadGroup) []*clusterv1alpha1.Cluster {
	// TODO: now support single spread constraint
	if len(spreadGroup.GroupRecord) > 1 {
		return nil
	}

	return g.chooseSpreadGroup(spreadGroup)
}

func chooseSpreadGroupByFieldCluster(
	spreadConstraint policyv1alpha1.SpreadConstraint,
	clusterGroups map[string]framework.ClusterScoreList) []*clusterv1alpha1.Cluster {
	if len(clusterGroups) < spreadConstraint.MinGroups {
		return nil
	}

	clusterScoreList := make(framework.ClusterScoreList, 0)
	for _, obj := range clusterGroups {
		clusterScoreList = append(clusterScoreList, obj...)
	}

	// prefer to choose the clusters with higher scores
	sort.Sort(clusterScoreList)

	var feasibleClusters []*clusterv1alpha1.Cluster
	var feasibleNum int
	if len(clusterGroups) <= spreadConstraint.MaxGroups {
		feasibleNum = len(clusterGroups)
	} else {
		feasibleNum = spreadConstraint.MaxGroups
	}

	for _, clusterScore := range clusterScoreList[0:feasibleNum] {
		feasibleClusters = append(feasibleClusters, clusterScore.Cluster)
	}

	klog.V(4).Infof("[spreadConstraints] select clusters : %v", feasibleClusters)
	return feasibleClusters
}

func (g *genericScheduler) chooseSpreadGroup(spreadGroup *util.SpreadGroup) []*clusterv1alpha1.Cluster {
	for spreadConstraint, clusterGroups := range spreadGroup.GroupRecord {
		// TODO: only support single spread constraint and the cluster constraint
		if spreadConstraint.SpreadByField == policyv1alpha1.SpreadByFieldCluster {
			return chooseSpreadGroupByFieldCluster(spreadConstraint, clusterGroups)
		}
	}
	return nil
}

func (g *genericScheduler) assignReplicas(
	clusters []*clusterv1alpha1.Cluster,
	replicaSchedulingStrategy *policyv1alpha1.ReplicaSchedulingStrategy,
	object *workv1alpha2.ResourceBindingSpec,
) ([]workv1alpha2.TargetCluster, error) {
	defer metrics.ScheduleStep(metrics.ScheduleStepAssignReplicas, time.Now())
	if len(clusters) == 0 {
		return nil, fmt.Errorf("no clusters available to schedule")
	}
	targetClusters := make([]workv1alpha2.TargetCluster, len(clusters))

	if object.Replicas > 0 && replicaSchedulingStrategy != nil {
		switch replicaSchedulingStrategy.ReplicaSchedulingType {
		// 1. Duplicated Scheduling
		case policyv1alpha1.ReplicaSchedulingTypeDuplicated:
			for i, cluster := range clusters {
				targetClusters[i] = workv1alpha2.TargetCluster{Name: cluster.Name, Replicas: object.Replicas}
			}
			return targetClusters, nil
		// 2. Divided Scheduling
		case policyv1alpha1.ReplicaSchedulingTypeDivided:
			switch replicaSchedulingStrategy.ReplicaDivisionPreference {
			// 2.1 Weighted Scheduling
			case policyv1alpha1.ReplicaDivisionPreferenceWeighted:
				// If ReplicaDivisionPreference is set to "Weighted" and WeightPreference is not set,
				// scheduler will weight all clusters averagely.
				if replicaSchedulingStrategy.WeightPreference == nil {
					replicaSchedulingStrategy.WeightPreference = getDefaultWeightPreference(clusters)
				}
				// 2.1.1 Dynamic Weighted Scheduling (by resource)
				if len(replicaSchedulingStrategy.WeightPreference.DynamicWeight) != 0 {
					return divideReplicasByDynamicWeight(clusters, replicaSchedulingStrategy.WeightPreference.DynamicWeight, object)
				}
				// 2.1.2 Static Weighted Scheduling
				return divideReplicasByStaticWeight(clusters, replicaSchedulingStrategy.WeightPreference.StaticWeightList, object.Replicas)
			// 2.2 Aggregated scheduling (by resource)
			case policyv1alpha1.ReplicaDivisionPreferenceAggregated:
				return divideReplicasByResource(clusters, object, policyv1alpha1.ReplicaDivisionPreferenceAggregated)
			default:
				return nil, fmt.Errorf("undefined replica division preference: %s", replicaSchedulingStrategy.ReplicaDivisionPreference)
			}
		default:
			return nil, fmt.Errorf("undefined replica scheduling type: %s", replicaSchedulingStrategy.ReplicaSchedulingType)
		}
	}

	for i, cluster := range clusters {
		targetClusters[i] = workv1alpha2.TargetCluster{Name: cluster.Name}
	}
	return targetClusters, nil
}
