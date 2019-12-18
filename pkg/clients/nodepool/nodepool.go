/*
Copyright 2019 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nodepool

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
	container "google.golang.org/api/container/v1beta1"

	"github.com/crossplaneio/stack-gcp/apis/container/v1beta1"
	gcp "github.com/crossplaneio/stack-gcp/pkg/clients"
)

const (
	// NodePoolNameFormat is the format for the fully qualified name of a node pool.
	NodePoolNameFormat = "%s/nodePools/%s"
)

// UpdateKind indicates the type of update needed for the node pool.
type UpdateKind int

// Set of possible cluster update kinds.
const (
	NoUpdate UpdateKind = iota
	AutoscalingUpdate
	ManagementUpdate
	SizeUpdate
	GeneralUpdate
)

// GenerateNodePool generates *container.NodePool instance from NodePoolParameters.
func GenerateNodePool(in v1beta1.NodePoolParameters, name string) *container.NodePool { // nolint:gocyclo
	pool := &container.NodePool{
		InitialNodeCount: gcp.Int64Value(in.InitialNodeCount),
		Locations:        in.Locations,
		Name:             name,
		Version:          gcp.StringValue(in.Version),
	}

	GenerateAutoscaling(in.Autoscaling, pool)
	GenerateConfig(in.Config, pool)
	GenerateManagement(in.Management, pool)
	GenerateMaxPodsConstraint(in.MaxPodsConstraint, pool)

	return pool
}

// GenerateAutoscaling generates *container.Autoscaling from *Autoscaling.
func GenerateAutoscaling(in *v1beta1.NodePoolAutoscaling, pool *container.NodePool) {
	if in != nil {
		out := &container.NodePoolAutoscaling{
			Autoprovisioned: gcp.BoolValue(in.Autoprovisioned),
			Enabled:         gcp.BoolValue(in.Enabled),
			MaxNodeCount:    gcp.Int64Value(in.MaxNodeCount),
			MinNodeCount:    gcp.Int64Value(in.MinNodeCount),
		}

		pool.Autoscaling = out
	}
}

// GenerateConfig generates *container.Config from *NodeConfig.
func GenerateConfig(in *v1beta1.NodeConfig, pool *container.NodePool) {
	if in != nil {
		out := &container.NodeConfig{
			DiskSizeGb:     gcp.Int64Value(in.DiskSizeGb),
			DiskType:       gcp.StringValue(in.DiskType),
			ImageType:      gcp.StringValue(in.ImageType),
			Labels:         in.Labels,
			LocalSsdCount:  gcp.Int64Value(in.LocalSsdCount),
			MachineType:    gcp.StringValue(in.MachineType),
			Metadata:       in.Metadata,
			MinCpuPlatform: gcp.StringValue(in.MinCPUPlatform),
			OauthScopes:    in.OauthScopes,
			Preemptible:    gcp.BoolValue(in.Preemptible),
			ServiceAccount: gcp.StringValue(in.ServiceAccount),
			Tags:           in.Tags,
		}

		for _, a := range in.Accelerators {
			if a != nil {
				out.Accelerators = append(out.Accelerators, &container.AcceleratorConfig{
					AcceleratorCount: a.AcceleratorCount,
					AcceleratorType:  a.AcceleratorType,
				})
			}
		}

		if in.SandboxConfig != nil {
			out.SandboxConfig = &container.SandboxConfig{
				SandboxType: in.SandboxConfig.SandboxType,
			}
		}

		if in.ShieldedInstanceConfig != nil {
			out.ShieldedInstanceConfig = &container.ShieldedInstanceConfig{
				EnableIntegrityMonitoring: gcp.BoolValue(in.ShieldedInstanceConfig.EnableIntegrityMonitoring),
				EnableSecureBoot:          gcp.BoolValue(in.ShieldedInstanceConfig.EnableSecureBoot),
			}
		}

		for _, t := range in.Taints {
			if t != nil {
				out.Taints = append(out.Taints, &container.NodeTaint{
					Effect: t.Effect,
					Key:    t.Key,
					Value:  t.Value,
				})
			}
		}

		if in.WorkloadMetadataConfig != nil {
			out.WorkloadMetadataConfig = &container.WorkloadMetadataConfig{
				NodeMetadata: in.WorkloadMetadataConfig.NodeMetadata,
			}
		}

		pool.Config = out
	}
}

// GenerateManagement generates *container.NodeManagement from *NodeManagementSpec.
func GenerateManagement(in *v1beta1.NodeManagementSpec, pool *container.NodePool) {
	if in != nil {
		out := &container.NodeManagement{
			AutoRepair:  gcp.BoolValue(in.AutoRepair),
			AutoUpgrade: gcp.BoolValue(in.AutoUpgrade),
		}

		pool.Management = out
	}
}

// GenerateMaxPodsConstraint generates *container.MaxPodsConstraint from *MaxPodsConstraint.
func GenerateMaxPodsConstraint(in *v1beta1.MaxPodsConstraint, pool *container.NodePool) {
	if in != nil {
		out := &container.MaxPodsConstraint{
			MaxPodsPerNode: in.MaxPodsPerNode,
		}

		pool.MaxPodsConstraint = out
	}
}

// GenerateObservation produces NodePoolObservation object from *container.NodePool object.
func GenerateObservation(in container.NodePool) v1beta1.NodePoolObservation { // nolint:gocyclo
	o := v1beta1.NodePoolObservation{
		InstanceGroupUrls: in.InstanceGroupUrls,
		PodIpv4CidrSize:   in.PodIpv4CidrSize,
		SelfLink:          in.SelfLink,
		Status:            in.Status,
		StatusMessage:     in.StatusMessage,
	}

	for _, condition := range in.Conditions {
		if condition != nil {
			o.Conditions = append(o.Conditions, &v1beta1.StatusCondition{
				Code:    condition.Code,
				Message: condition.Message,
			})
		}
	}

	if in.Management != nil && in.Management.UpgradeOptions != nil {
		o.Management = &v1beta1.NodeManagementStatus{
			UpgradeOptions: &v1beta1.AutoUpgradeOptions{
				AutoUpgradeStartTime: in.Management.UpgradeOptions.AutoUpgradeStartTime,
				Description:          in.Management.UpgradeOptions.Description,
			},
		}
	}

	return o

}

// GenerateNodePoolUpdate produces NodePoolObservation object from *container.NodePool object.
func GenerateNodePoolUpdate(in *v1beta1.NodePoolParameters) *container.UpdateNodePoolRequest { // nolint:gocyclo
	o := &container.UpdateNodePoolRequest{
		Locations:   in.Locations,
		NodeVersion: gcp.StringValue(in.Version),
	}

	if in.Config != nil {
		o.ImageType = gcp.StringValue(in.Config.ImageType)

		if in.Config.WorkloadMetadataConfig != nil {
			o.WorkloadMetadataConfig = &container.WorkloadMetadataConfig{
				NodeMetadata: in.Config.WorkloadMetadataConfig.NodeMetadata,
			}
		}
	}

	return o
}

// LateInitializeSpec fills unassigned fields with the values in container.NodePool object.
func LateInitializeSpec(spec *v1beta1.NodePoolParameters, in container.NodePool) { // nolint:gocyclo
	if in.Autoscaling != nil {
		if spec.Autoscaling == nil {
			spec.Autoscaling = &v1beta1.NodePoolAutoscaling{}
		}

		spec.Autoscaling.Autoprovisioned = gcp.LateInitializeBool(spec.Autoscaling.Autoprovisioned, in.Autoscaling.Autoprovisioned)
		spec.Autoscaling.Enabled = gcp.LateInitializeBool(spec.Autoscaling.Enabled, in.Autoscaling.Enabled)
		spec.Autoscaling.MaxNodeCount = gcp.LateInitializeInt64(spec.Autoscaling.MaxNodeCount, in.Autoscaling.MaxNodeCount)
		spec.Autoscaling.MinNodeCount = gcp.LateInitializeInt64(spec.Autoscaling.MinNodeCount, in.Autoscaling.MinNodeCount)
	}

	if in.Config != nil {
		if spec.Config == nil {
			spec.Config = &v1beta1.NodeConfig{}
		}

		if len(in.Config.Accelerators) != 0 && len(spec.Config.Accelerators) == 0 {
			spec.Config.Accelerators = make([]*v1beta1.AcceleratorConfig, len(in.Config.Accelerators))
			for i, a := range in.Config.Accelerators {
				spec.Config.Accelerators[i] = &v1beta1.AcceleratorConfig{
					AcceleratorCount: a.AcceleratorCount,
					AcceleratorType:  a.AcceleratorType,
				}
			}
		}

		spec.Config.DiskSizeGb = gcp.LateInitializeInt64(spec.Config.DiskSizeGb, in.Config.DiskSizeGb)
		spec.Config.DiskType = gcp.LateInitializeString(spec.Config.DiskType, in.Config.DiskType)
		spec.Config.ImageType = gcp.LateInitializeString(spec.Config.ImageType, in.Config.ImageType)
		spec.Config.Labels = gcp.LateInitializeStringMap(spec.Config.Labels, in.Config.Labels)
		spec.Config.LocalSsdCount = gcp.LateInitializeInt64(spec.Config.LocalSsdCount, in.Config.LocalSsdCount)
		spec.Config.MachineType = gcp.LateInitializeString(spec.Config.MachineType, in.Config.MachineType)
		spec.Config.Metadata = gcp.LateInitializeStringMap(spec.Config.Metadata, in.Config.Metadata)
		spec.Config.MinCPUPlatform = gcp.LateInitializeString(spec.Config.MinCPUPlatform, in.Config.MinCpuPlatform)
		spec.Config.OauthScopes = gcp.LateInitializeStringSlice(spec.Config.OauthScopes, in.Config.OauthScopes)
		spec.Config.Preemptible = gcp.LateInitializeBool(spec.Config.Preemptible, in.Config.Preemptible)

		if in.Config.SandboxConfig != nil && spec.Config.SandboxConfig == nil {
			spec.Config.SandboxConfig = &v1beta1.SandboxConfig{
				SandboxType: in.Config.SandboxConfig.SandboxType,
			}
		}

		spec.Config.ServiceAccount = gcp.LateInitializeString(spec.Config.ServiceAccount, in.Config.ServiceAccount)

		if in.Config.ShieldedInstanceConfig != nil {
			if spec.Config.ShieldedInstanceConfig == nil {
				spec.Config.ShieldedInstanceConfig = &v1beta1.ShieldedInstanceConfig{}
			}
			spec.Config.ShieldedInstanceConfig.EnableIntegrityMonitoring = gcp.LateInitializeBool(spec.Config.ShieldedInstanceConfig.EnableIntegrityMonitoring, in.Config.ShieldedInstanceConfig.EnableIntegrityMonitoring)
			spec.Config.ShieldedInstanceConfig.EnableSecureBoot = gcp.LateInitializeBool(spec.Config.ShieldedInstanceConfig.EnableSecureBoot, in.Config.ShieldedInstanceConfig.EnableSecureBoot)
		}

		spec.Config.Tags = gcp.LateInitializeStringSlice(spec.Config.Tags, in.Config.Tags)

		if len(in.Config.Taints) != 0 && len(spec.Config.Taints) == 0 {
			spec.Config.Taints = make([]*v1beta1.NodeTaint, len(in.Config.Taints))
			for i, t := range in.Config.Taints {
				spec.Config.Taints[i] = &v1beta1.NodeTaint{
					Effect: t.Effect,
					Key:    t.Key,
					Value:  t.Value,
				}
			}
		}

		if in.Config.WorkloadMetadataConfig != nil && spec.Config.WorkloadMetadataConfig == nil {
			spec.Config.WorkloadMetadataConfig = &v1beta1.WorkloadMetadataConfig{
				NodeMetadata: in.Config.WorkloadMetadataConfig.NodeMetadata,
			}
		}
	}

	spec.InitialNodeCount = gcp.LateInitializeInt64(spec.InitialNodeCount, in.InitialNodeCount)
	spec.Locations = gcp.LateInitializeStringSlice(spec.Locations, in.Locations)

	if in.Management != nil {
		if spec.Management == nil {
			spec.Management = &v1beta1.NodeManagementSpec{}
		}

		spec.Management.AutoRepair = gcp.LateInitializeBool(spec.Management.AutoRepair, in.Management.AutoRepair)
		spec.Management.AutoUpgrade = gcp.LateInitializeBool(spec.Management.AutoUpgrade, in.Management.AutoUpgrade)
	}

	if in.MaxPodsConstraint != nil && spec.MaxPodsConstraint == nil {
		spec.MaxPodsConstraint = &v1beta1.MaxPodsConstraint{
			MaxPodsPerNode: in.MaxPodsConstraint.MaxPodsPerNode,
		}
	}

	spec.Version = gcp.LateInitializeString(spec.Version, in.Version)
}

// IsUpToDate checks whether current state is up-to-date compared to the given
// set of parameters.
func IsUpToDate(in *v1beta1.NodePoolParameters, currentState container.NodePool) (bool, UpdateKind) {
	currentParams := &v1beta1.NodePoolParameters{}
	LateInitializeSpec(currentParams, currentState)
	if !cmp.Equal(in.Autoscaling, currentParams.Autoscaling) {
		return false, AutoscalingUpdate
	}
	if !cmp.Equal(in.Management, currentParams.Management) {
		return false, ManagementUpdate
	}
	// if !cmp.Equal(in.InitialNodeCount, currentParams.InitialNodeCount) {
	// 	return false, AutoscalingUpdate
	// }
	if !cmp.Equal(in, currentParams) {
		return false, GeneralUpdate
	}
	return true, NoUpdate
}

// GetFullyQualifiedName builds the fully qualified name of the cluster.
func GetFullyQualifiedName(p v1beta1.NodePoolParameters, name string) string {
	return fmt.Sprintf(NodePoolNameFormat, gcp.StringValue(p.Cluster), name)
}