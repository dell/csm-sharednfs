/*
Copyright © 2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package nfs

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
)

func TestGetPluginInfo(t *testing.T) {
	ids := &CsiNfsService{}
	req := &csi.GetPluginInfoRequest{}
	ctx := context.Background()

	resp, err := ids.GetPluginInfo(ctx, req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "your-plugin-name", resp.Name)
	assert.
		Equal(t, "0.1.0", resp.VendorVersion)
}

func TestGetPluginCapabilities(t *testing.T) {
	ids := &CsiNfsService{}
	req := &csi.GetPluginCapabilitiesRequest{}
	ctx := context.Background()

	resp, err := ids.GetPluginCapabilities(ctx, req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, resp.Capabilities, 1)
	assert.Equal(t, csi.PluginCapability_Service_CONTROLLER_SERVICE, resp.Capabilities[0].GetService().GetType())
}

func TestProbe(t *testing.T) {
	ids := &CsiNfsService{}
	req := &csi.ProbeRequest{}
	ctx := context.Background()

	resp, err := ids.Probe(ctx, req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}
