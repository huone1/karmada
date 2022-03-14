package core

import (
	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	workv1alpha2 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha2"
	"github.com/karmada-io/karmada/pkg/scheduler/framework"
	"sort"
)

type ClusterDetailInfos []ClusterDetailInfo

func (a ClusterDetailInfos) Len() int           { return len(a) }
func (a ClusterDetailInfos) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ClusterDetailInfos) Less(i, j int) bool { return a[i].Score > a[j].Score }

type ZoneInfos []ZoneInfo

func (a ZoneInfos) Len() int           { return len(a) }
func (a ZoneInfos) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ZoneInfos) Less(i, j int) bool { return a[i].Score > a[j].Score }

type RegionInfos []RegionInfo

func (a RegionInfos) Len() int           { return len(a) }
func (a RegionInfos) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a RegionInfos) Less(i, j int) bool { return a[i].Score > a[j].Score }

type ProviderInfos []ProviderInfo

func (a ProviderInfos) Len() int           { return len(a) }
func (a ProviderInfos) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ProviderInfos) Less(i, j int) bool { return a[i].Score > a[j].Score }

type GroupClustersInfo struct {
	// Providers from globally view, sorted by providers.Score descending.
	Providers []ProviderInfo
	// Regions from globally view, sorted by region.Score descending.
	Regions []RegionInfo
	// Zones from globally view, sorted by zone.Score descending.
	Zones []ZoneInfo
	// Clusters from globally view, sorted by cluster.Score descending.
	Clusters []ClusterDetailInfo
}

type ProviderInfo struct {
	Name              string
	Score             int64
	AvailableReplicas int64

	Regions []RegionInfo
	Zones   []ZoneInfo
	// Clusters under this provider, sorted by cluster.Score descending.
	Clusters []ClusterDetailInfo
}

type RegionInfo struct {
	Name              string
	Score             int64
	AvailableReplicas int64

	ProviderName string

	// Zones under this region, sorted by zone.Score descending.
	Zones []ZoneInfo
	// Clusters under this region, sorted by cluster.Score descending.
	Clusters []ClusterDetailInfo
}

type ZoneInfo struct {
	Name              string
	Score             int64
	AvailableReplicas int64

	RegionName   string
	ProviderName string

	// Clusters under this zone, sorted by cluster.Score descending.
	Clusters []ClusterDetailInfo
}

type ClusterDetailInfo struct {
	Name              string
	Score             int64
	AvailableReplicas int64

	Cluster *clusterv1alpha1.Cluster
}

func GroupClustersWithScore(
	clustersScore framework.ClusterScoreList,
	placement *policyv1alpha1.Placement,
	spec *workv1alpha2.ResourceBindingSpec,
) *GroupClustersInfo {
	if IsJustConcernedCluster(placement) {
		return groupClustersIngoreTopology(clustersScore, spec)
	}

	return groupClustersBasedTopology(clustersScore, spec)
}

func groupClustersBasedTopology(
	clustersScore framework.ClusterScoreList,
	rbSpec *workv1alpha2.ResourceBindingSpec,
) *GroupClustersInfo {
	groupClustersInfo := &GroupClustersInfo{}
	groupClustersInfo.generateClustersInfo(clustersScore, rbSpec)
	groupClustersInfo.generateZoneInfo()
	groupClustersInfo.generateRegionInfo()
	groupClustersInfo.generateProviderInfo()

	return groupClustersInfo
}

func groupClustersIngoreTopology(
	clustersScore framework.ClusterScoreList,
	rbSpec *workv1alpha2.ResourceBindingSpec,
) *GroupClustersInfo {
	groupClustersInfo := &GroupClustersInfo{}
	groupClustersInfo.generateClustersInfo(clustersScore, rbSpec)

	return groupClustersInfo
}

func (info *GroupClustersInfo) generateClustersInfo(clustersScore framework.ClusterScoreList, rbSpec *workv1alpha2.ResourceBindingSpec) {
	var clusters []*clusterv1alpha1.Cluster
	for _, clusterScore := range clustersScore {
		clusterInfo := ClusterDetailInfo{}
		clusterInfo.Name = clusterScore.Cluster.Name
		clusterInfo.Score = clusterScore.Score
		clusterInfo.Cluster = clusterScore.Cluster
		info.Clusters = append(info.Clusters, clusterInfo)
		clusters = append(clusters, clusterScore.Cluster)
	}

	clustersRaplicas := calAvailableReplicas(clusters, rbSpec)
	for i, clustersRaplica := range clustersRaplicas {
		info.Clusters[i].AvailableReplicas = int64(clustersRaplica.Replicas)
	}

	sort.Sort(ClusterDetailInfos(info.Clusters))
}

func (info *GroupClustersInfo) generateZoneInfo() {
	zoneInfoMap := make(map[string]ZoneInfo)

	for _, clusterInfo := range info.Clusters {
		zone := clusterInfo.Cluster.Spec.Zone
		zoneInfo, ok := zoneInfoMap[zone]
		if !ok {
			zoneInfo = ZoneInfo{
				Name:         zone,
				RegionName:   clusterInfo.Cluster.Spec.Region,
				ProviderName: clusterInfo.Cluster.Spec.Provider,
				Clusters:     make([]ClusterDetailInfo, 0),
			}
		}

		zoneInfo.Clusters = append(zoneInfo.Clusters, clusterInfo)
		zoneInfo.Score += clusterInfo.Score
		zoneInfo.AvailableReplicas += clusterInfo.AvailableReplicas
		zoneInfoMap[zone] = zoneInfo
	}

	for _, val := range zoneInfoMap {
		sort.Sort(ClusterDetailInfos(val.Clusters))
		info.Zones = append(info.Zones, val)
	}

	sort.Sort(ZoneInfos(info.Zones))

}

func (info *GroupClustersInfo) generateRegionInfo() {
	regionInfoMap := make(map[string]RegionInfo)

	for _, zoneInfo := range info.Zones {
		regionInfo, ok := regionInfoMap[zoneInfo.RegionName]
		if !ok {
			regionInfo = RegionInfo{
				Name:         zoneInfo.RegionName,
				ProviderName: zoneInfo.ProviderName,
				Zones:        make([]ZoneInfo, 0),
				Clusters:     make([]ClusterDetailInfo, 0),
			}
		}

		regionInfo.Score += zoneInfo.Score
		regionInfo.AvailableReplicas += zoneInfo.AvailableReplicas
		regionInfo.Zones = append(regionInfo.Zones, zoneInfo)
		regionInfo.Clusters = append(regionInfo.Clusters, zoneInfo.Clusters...)
		regionInfoMap[zoneInfo.RegionName] = regionInfo
	}

	for _, val := range regionInfoMap {
		sort.Sort(ClusterDetailInfos(val.Clusters))
		sort.Sort(ZoneInfos(val.Zones))
		info.Regions = append(info.Regions, val)
	}

	sort.Sort(RegionInfos(info.Regions))
}

func (info *GroupClustersInfo) generateProviderInfo() {
	providerInfoMap := make(map[string]ProviderInfo)

	for _, regionInfo := range info.Regions {
		providerInfo, ok := providerInfoMap[regionInfo.ProviderName]
		if !ok {
			providerInfo = ProviderInfo{
				Name:     regionInfo.ProviderName,
				Regions:  make([]RegionInfo, 0),
				Zones:    make([]ZoneInfo, 0),
				Clusters: make([]ClusterDetailInfo, 0),
			}
		}

		providerInfo.Score += regionInfo.Score
		providerInfo.AvailableReplicas += regionInfo.AvailableReplicas
		providerInfo.Regions = append(providerInfo.Regions, regionInfo)
		providerInfo.Zones = append(providerInfo.Zones, regionInfo.Zones...)
		providerInfo.Clusters = append(providerInfo.Clusters, regionInfo.Clusters...)
		providerInfoMap[regionInfo.ProviderName] = providerInfo
	}

	for _, val := range providerInfoMap {
		sort.Sort(ClusterDetailInfos(val.Clusters))
		sort.Sort(ZoneInfos(val.Zones))
		sort.Sort(RegionInfos(val.Regions))
		info.Providers = append(info.Providers, val)
	}
}
