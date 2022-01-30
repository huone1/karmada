package util

import (
	"sync"

	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	"github.com/karmada-io/karmada/pkg/scheduler/framework"
)

// SpreadGroup stores the cluster group info for given spread constraints
type SpreadGroup struct {
	// The outer map's keys are SpreadConstraint. The values (inner map) of the outer map are maps with string
	// keys and []string values. The inner map's key should specify the cluster group name.
	GroupRecord map[policyv1alpha1.SpreadConstraint]map[string]framework.ClusterScoreList
	sync.RWMutex
}

// NewSpreadGroup initializes a SpreadGroup
func NewSpreadGroup() *SpreadGroup {
	return &SpreadGroup{
		GroupRecord: make(map[policyv1alpha1.SpreadConstraint]map[string]framework.ClusterScoreList),
	}
}

// InitialGroupRecord initials a spread state record
func (ss *SpreadGroup) InitialGroupRecord(constraint policyv1alpha1.SpreadConstraint) {
	ss.Lock()
	defer ss.Unlock()
	ss.GroupRecord[constraint] = make(map[string]framework.ClusterScoreList)
}
