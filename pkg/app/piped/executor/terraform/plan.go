// Copyright 2020 The PipeCD Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package terraform

import (
	"context"
	"path/filepath"

	provider "github.com/pipe-cd/pipe/pkg/app/piped/cloudprovider/terraform"
	"github.com/pipe-cd/pipe/pkg/model"
)

func (e *Executor) ensurePlan(ctx context.Context) model.StageStatus {
	appDir := filepath.Join(e.RepoDir, e.Deployment.GitPath.Path)
	cmd := provider.NewTerraform(e.terraformPath, appDir, e.vars, e.config.Input.VarFiles)

	if ok := e.showUsingVersion(ctx, cmd); !ok {
		return model.StageStatus_STAGE_FAILURE
	}

	if err := cmd.Init(ctx, e.LogPersister); err != nil {
		e.LogPersister.Errorf("Failed to init (%v)", err)
		return model.StageStatus_STAGE_FAILURE
	}

	if ok := e.selectWorkspace(ctx, cmd); !ok {
		return model.StageStatus_STAGE_FAILURE
	}

	planResult, err := cmd.Plan(ctx, e.LogPersister)
	if err != nil {
		e.LogPersister.Errorf("Failed to plan (%v)", err)
		return model.StageStatus_STAGE_FAILURE
	}

	if planResult.NoChanges() {
		e.LogPersister.Success("No changes to apply")
		return model.StageStatus_STAGE_SUCCESS
	}

	e.LogPersister.Successf("Detected %d add, %d change, %d destroy.", planResult.Adds, planResult.Changes, planResult.Destroys)
	return model.StageStatus_STAGE_SUCCESS
}
