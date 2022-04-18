package spreadconstraint

import (
	"reflect"
	"testing"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
)

func Test_selectBestClustersByRegion(t *testing.T) {
	type args struct {
		spreadConstraints []policyv1alpha1.SpreadConstraint
		groupClustersInfo *GroupClustersInfo
	}
	tests := []struct {
		name    string
		args    args
		want    []*clusterv1alpha1.Cluster
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectBestClustersByRegion(tt.args.spreadConstraints, tt.args.groupClustersInfo)
			if (err != nil) != tt.wantErr {
				t.Errorf("selectBestClustersByRegion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("selectBestClustersByRegion() = %v, want %v", got, tt.want)
			}
		})
	}
}
