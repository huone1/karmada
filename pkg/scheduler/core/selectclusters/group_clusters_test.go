package selectclusters

import (
	"testing"

	"github.com/agiledragon/gomonkey/v2"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	workv1alpha2 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha2"
	"github.com/karmada-io/karmada/pkg/scheduler/framework"
)

func generateClusterScore() framework.ClusterScoreList {
	return framework.ClusterScoreList{
		{
			Cluster: NewClusterWithTopology("member1", "P1", "R1", "Z1"),
			Score:   20,
		},
		{
			Cluster: NewClusterWithTopology("member2", "P1", "R1", "Z2"),
			Score:   40,
		},
		{
			Cluster: NewClusterWithTopology("member3", "P2", "R1", "Z1"),
			Score:   30,
		},
		{
			Cluster: NewClusterWithTopology("member4", "P2", "R2", "Z2"),
			Score:   60,
		},
	}
}
func Test_GroupClustersWithScore(t *testing.T) {
	type args struct {
		clustersScore framework.ClusterScoreList
		placement     *policyv1alpha1.Placement
		spec          *workv1alpha2.ResourceBindingSpec
	}

	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "test SpreadConstraints is nil",
			args: args{
				clustersScore: generateClusterScore(),
				placement:     &policyv1alpha1.Placement{},
			},
			want: []string{"member4", "member2", "member3", "member1"},
		},
		{
			name: "test SpreadConstraints is cluster",
			args: args{
				clustersScore: generateClusterScore(),
				placement: &policyv1alpha1.Placement{
					SpreadConstraints: []policyv1alpha1.SpreadConstraint{
						{
							SpreadByField: policyv1alpha1.SpreadByFieldCluster,
							MaxGroups:     1,
							MinGroups:     1,
						},
					},
				},
			},
			want: []string{"member4", "member2", "member3", "member1"},
		},
	}

	patches := gomonkey.ApplyFunc(calAvailableReplicas, func(clusters []*clusterv1alpha1.Cluster, spec *workv1alpha2.ResourceBindingSpec) []workv1alpha2.TargetCluster {
		availableTargetClusters := make([]workv1alpha2.TargetCluster, len(clusters))

		for i := range availableTargetClusters {
			availableTargetClusters[i].Name = clusters[i].Name
			availableTargetClusters[i].Replicas = 100
		}

		return availableTargetClusters
	})

	defer patches.Reset()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groupInfo := GroupClustersWithScore(tt.args.clustersScore, tt.args.placement, tt.args.spec)
			for i, cluster := range groupInfo.Clusters {
				if cluster.Name != tt.want[i] {
					t.Errorf("test %s : the clusters aren't sorted", tt.name)
				}
			}
		})
	}
}
