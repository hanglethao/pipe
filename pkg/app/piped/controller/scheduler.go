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

package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/pipe-cd/pipe/pkg/app/api/service/pipedservice"
	"github.com/pipe-cd/pipe/pkg/app/piped/executor"
	"github.com/pipe-cd/pipe/pkg/app/piped/executor/registry"
	"github.com/pipe-cd/pipe/pkg/app/piped/logpersister"
	pln "github.com/pipe-cd/pipe/pkg/app/piped/planner"
	"github.com/pipe-cd/pipe/pkg/cache"
	"github.com/pipe-cd/pipe/pkg/config"
	"github.com/pipe-cd/pipe/pkg/model"
)

var (
	workspaceGitRepoDirName        = "repo"
	workspaceGitRunningRepoDirName = "running-repo"
	workspaceStagesDirName         = "stages"
	defaultDeploymentTimeout       = time.Hour
)

// scheduler is a dedicated object for a specific deployment of a single application.
type scheduler struct {
	// Readonly deployment model.
	deployment            *model.Deployment
	envName               string
	workingDir            string
	executorRegistry      registry.Registry
	apiClient             apiClient
	gitClient             gitClient
	commandLister         commandLister
	applicationLister     applicationLister
	liveResourceLister    liveResourceLister
	logPersister          logpersister.Persister
	metadataStore         *metadataStore
	notifier              notifier
	sealedSecretDecrypter sealedSecretDecrypter
	pipedConfig           *config.PipedSpec
	appManifestsCache     cache.Cache
	logger                *zap.Logger

	deploymentConfig      *config.Config
	genericDeploymentSpec config.GenericDeploymentSpec
	prepareMu             sync.Mutex
	prepared              bool
	// Current status of each stages.
	// We stores their current statuses into this field
	// because the deployment model is readonly to avoid data race.
	// We may need a mutex for this field in the future
	// when the stages can be executed concurrently.
	stageStatuses map[string]model.StageStatus

	done                 atomic.Bool
	doneTimestamp        time.Time
	doneDeploymentStatus model.DeploymentStatus
	cancelled            bool
	cancelledCh          chan *model.ReportableCommand

	nowFunc func() time.Time
}

func newScheduler(
	d *model.Deployment,
	envName string,
	workingDir string,
	apiClient apiClient,
	gitClient gitClient,
	commandLister commandLister,
	applicationLister applicationLister,
	liveResourceLister liveResourceLister,
	lp logpersister.Persister,
	notifier notifier,
	ssd sealedSecretDecrypter,
	pipedConfig *config.PipedSpec,
	appManifestsCache cache.Cache,
	logger *zap.Logger,
) *scheduler {

	logger = logger.Named("scheduler").With(
		zap.String("deployment-id", d.Id),
		zap.String("application-id", d.ApplicationId),
		zap.String("env-id", d.EnvId),
		zap.String("project-id", d.ProjectId),
		zap.String("application-kind", d.Kind.String()),
		zap.String("working-dir", workingDir),
	)

	s := &scheduler{
		deployment:            d,
		envName:               envName,
		workingDir:            workingDir,
		executorRegistry:      registry.DefaultRegistry(),
		apiClient:             apiClient,
		gitClient:             gitClient,
		commandLister:         commandLister,
		applicationLister:     applicationLister,
		liveResourceLister:    liveResourceLister,
		logPersister:          lp,
		metadataStore:         NewMetadataStore(apiClient, d),
		notifier:              notifier,
		sealedSecretDecrypter: ssd,
		pipedConfig:           pipedConfig,
		appManifestsCache:     appManifestsCache,
		doneDeploymentStatus:  d.Status,
		cancelledCh:           make(chan *model.ReportableCommand, 1),
		logger:                logger,
		nowFunc:               time.Now,
	}

	// Initialize the map of current status of all stages.
	s.stageStatuses = make(map[string]model.StageStatus, len(d.Stages))
	for _, stage := range d.Stages {
		s.stageStatuses[stage.Id] = stage.Status
	}

	return s
}

// ID returns the id of scheduler.
// This is the same value with deployment ID.
func (s *scheduler) ID() string {
	return s.deployment.Id
}

// CommitHash returns the hash value of deploying commit.
func (s *scheduler) CommitHash() string {
	return s.deployment.CommitHash()
}

// IsDone tells whether this scheduler is done it tasks or not.
// Returning true means this scheduler can be removable.
func (s *scheduler) IsDone() bool {
	return s.done.Load()
}

// DoneTimestamp returns the time when scheduler has done.
// This can be used only after IsDone() returns true.
func (s *scheduler) DoneTimestamp() time.Time {
	if !s.IsDone() {
		return time.Now().AddDate(1, 0, 0)
	}
	return s.doneTimestamp
}

// DoneDeploymentStatus returns the deployment status when scheduler has done.
// This can be used only after IsDone() returns true.
func (s *scheduler) DoneDeploymentStatus() model.DeploymentStatus {
	if !s.IsDone() {
		return s.deployment.Status
	}
	return s.doneDeploymentStatus
}

func (s *scheduler) Cancel(cmd model.ReportableCommand) {
	if s.cancelled {
		return
	}
	s.cancelled = true
	s.cancelledCh <- &cmd
	close(s.cancelledCh)
}

// Run starts running the scheduler.
// It determines what stage should be executed next by which executor.
// The returning error does not mean that the pipeline was failed,
// but it means that the scheduler could not finish its job normally.
func (s *scheduler) Run(ctx context.Context) error {
	s.logger.Info("start running a scheduler")
	defer func() {
		s.doneTimestamp = s.nowFunc()
		s.done.Store(true)
	}()

	// If this deployment is already completed. Do nothing.
	if model.IsCompletedDeployment(s.deployment.Status) {
		s.logger.Info("this deployment is already completed")
		return nil
	}

	// Update deployment status to RUNNING if needed.
	if model.CanUpdateDeploymentStatus(s.deployment.Status, model.DeploymentStatus_DEPLOYMENT_RUNNING) {
		err := s.reportDeploymentStatusChanged(ctx, model.DeploymentStatus_DEPLOYMENT_RUNNING, "The piped started handling this deployment")
		if err != nil {
			return err
		}
	}

	var (
		deploymentStatus = model.DeploymentStatus_DEPLOYMENT_SUCCESS
		statusReason     = "The deployment was completed successfully"
		timer            = time.NewTimer(defaultDeploymentTimeout)
		cancelCommand    *model.ReportableCommand
		cancelCommander  string
		lastStage        *model.PipelineStage
	)
	defer timer.Stop()

	// Iterate all the stages and execute the uncompleted ones.
	for i, ps := range s.deployment.Stages {
		lastStage = s.deployment.Stages[i]

		if ps.Status == model.StageStatus_STAGE_SUCCESS {
			continue
		}
		if !ps.Visible || ps.Name == model.StageRollback.String() {
			continue
		}

		// This stage is already completed by a previous scheduler.
		if ps.Status == model.StageStatus_STAGE_CANCELLED {
			deploymentStatus = model.DeploymentStatus_DEPLOYMENT_CANCELLED
			statusReason = fmt.Sprintf("Deployment was cancelled while executing stage %s", ps.Id)
			break
		}
		if ps.Status == model.StageStatus_STAGE_FAILURE {
			deploymentStatus = model.DeploymentStatus_DEPLOYMENT_FAILURE
			statusReason = fmt.Sprintf("Failed while executing stage %s", ps.Id)
			break
		}

		var (
			result       model.StageStatus
			sig, handler = executor.NewStopSignal()
			doneCh       = make(chan struct{})
		)

		go func() {
			result = s.executeStage(sig, *ps, func(in executor.Input) (executor.Executor, bool) {
				return s.executorRegistry.Executor(model.Stage(ps.Name), in)
			})
			close(doneCh)
		}()

		select {
		case <-ctx.Done():
			handler.Terminate()
			<-doneCh

		case <-timer.C:
			handler.Timeout()
			<-doneCh

		case cmd := <-s.cancelledCh:
			if cmd != nil {
				cancelCommand = cmd
				cancelCommander = cmd.Commander
				handler.Cancel()
				<-doneCh
			}

		case <-doneCh:
			break
		}

		// If all operations of the stage were completed successfully
		// go the next stage to handle.
		if result == model.StageStatus_STAGE_SUCCESS {
			continue
		}

		sigType := sig.Signal()

		// The deployment was cancelled by a web user.
		if sigType == executor.StopSignalCancel {
			deploymentStatus = model.DeploymentStatus_DEPLOYMENT_CANCELLED
			statusReason = fmt.Sprintf("Deployment was cancelled by %s while executing stage %s", cancelCommander, ps.Id)
			break
		}

		// The stage was failed but not caused by the stop signal.
		if result == model.StageStatus_STAGE_FAILURE && sigType == executor.StopSignalNone {
			deploymentStatus = model.DeploymentStatus_DEPLOYMENT_FAILURE
			statusReason = fmt.Sprintf("Failed while executing stage %s", ps.Id)
			break
		}

		return nil
	}

	// When the deployment has completed but not successful,
	// we start rollback stage if the auto-rollback option is true.
	if deploymentStatus == model.DeploymentStatus_DEPLOYMENT_CANCELLED ||
		deploymentStatus == model.DeploymentStatus_DEPLOYMENT_FAILURE {
		if stage, ok := s.deployment.FindRollbackStage(); ok {
			// Update to change deployment status to ROLLING_BACK.
			if err := s.reportDeploymentStatusChanged(ctx, model.DeploymentStatus_DEPLOYMENT_ROLLING_BACK, statusReason); err != nil {
				return err
			}

			// Start running rollback stage.
			var (
				sig, handler = executor.NewStopSignal()
				doneCh       = make(chan struct{})
			)
			go func() {
				rbs := *stage
				rbs.Requires = []string{lastStage.Id}
				s.executeStage(sig, rbs, func(in executor.Input) (executor.Executor, bool) {
					return s.executorRegistry.RollbackExecutor(s.deployment.Kind, in)
				})
				close(doneCh)
			}()

			select {
			case <-ctx.Done():
				handler.Terminate()
				<-doneCh
				return nil

			case <-doneCh:
				break
			}
		}
	}

	if model.IsCompletedDeployment(deploymentStatus) {
		err := s.reportDeploymentCompleted(ctx, deploymentStatus, statusReason, cancelCommander)
		if err == nil && deploymentStatus == model.DeploymentStatus_DEPLOYMENT_SUCCESS {
			s.reportMostRecentlySuccessfulDeployment(ctx)
		}
		s.doneDeploymentStatus = deploymentStatus
	}

	if cancelCommand != nil {
		if err := cancelCommand.Report(ctx, model.CommandStatus_COMMAND_SUCCEEDED, nil); err != nil {
			s.logger.Error("failed to report command status", zap.Error(err))
		}
	}

	return nil
}

// executeStage finds the executor for the given stage and execute.
func (s *scheduler) executeStage(sig executor.StopSignal, ps model.PipelineStage, executorFactory func(executor.Input) (executor.Executor, bool)) model.StageStatus {
	var (
		ctx            = sig.Context()
		originalStatus = ps.Status
		lp             = s.logPersister.StageLogPersister(s.deployment.Id, ps.Id)
	)
	defer lp.Complete(time.Minute)

	// Update stage status to RUNNING if needed.
	if model.CanUpdateStageStatus(ps.Status, model.StageStatus_STAGE_RUNNING) {
		if err := s.reportStageStatus(ctx, ps.Id, model.StageStatus_STAGE_RUNNING, ps.Requires); err != nil {
			return model.StageStatus_STAGE_FAILURE
		}
		originalStatus = model.StageStatus_STAGE_RUNNING
	}

	// Check the existence of the specified cloud provider.
	if !s.pipedConfig.HasCloudProvider(s.deployment.CloudProvider, s.deployment.CloudProviderType()) {
		lp.Errorf("This piped is not having the specified cloud provider in this deployment: %v", s.deployment.CloudProvider)
		if err := s.reportStageStatus(ctx, ps.Id, model.StageStatus_STAGE_FAILURE, ps.Requires); err != nil {
			s.logger.Error("failed to report stage status", zap.Error(err))
		}
		return model.StageStatus_STAGE_FAILURE
	}

	// Ensure that all needed things has been prepared before executing any stage.
	var (
		needTargetCommit  = ps.Name != model.StageRollback.String()
		needRunningCommit = s.deployment.RunningCommitHash != ""
	)
	if err := s.ensurePreparing(ctx, needTargetCommit, needRunningCommit, lp); err != nil {
		if !sig.Stopped() {
			if err := s.reportStageStatus(ctx, ps.Id, model.StageStatus_STAGE_FAILURE, ps.Requires); err != nil {
				s.logger.Error("failed to report stage status", zap.Error(err))
			}
			return model.StageStatus_STAGE_FAILURE
		}
		return originalStatus
	}

	var stageConfig *config.PipelineStage
	if !ps.Predefined {
		if sc, ok := s.genericDeploymentSpec.GetStage(ps.Index); ok {
			stageConfig = &sc
		}
	} else {
		if sc, ok := pln.GetPredefinedStage(ps.Id); ok {
			stageConfig = &sc
		}
	}
	if stageConfig == nil {
		lp.Error("Unable to find the stage configuration")
		if err := s.reportStageStatus(ctx, ps.Id, model.StageStatus_STAGE_FAILURE, ps.Requires); err != nil {
			s.logger.Error("failed to report stage status", zap.Error(err))
		}
		return model.StageStatus_STAGE_FAILURE
	}

	app, ok := s.applicationLister.Get(s.deployment.ApplicationId)
	if !ok {
		lp.Errorf("Application %s for this deployment was not found (Maybe it was disabled).", s.deployment.ApplicationId)
		s.reportStageStatus(ctx, ps.Id, model.StageStatus_STAGE_FAILURE, ps.Requires)
		return model.StageStatus_STAGE_FAILURE
	}

	input := executor.Input{
		Stage:            &ps,
		StageConfig:      *stageConfig,
		Deployment:       s.deployment,
		DeploymentConfig: s.deploymentConfig,
		PipedConfig:      s.pipedConfig,
		Application:      app,
		WorkingDir:       s.workingDir,
		RepoDir:          filepath.Join(s.workingDir, workspaceGitRepoDirName),
		RunningRepoDir:   filepath.Join(s.workingDir, workspaceGitRunningRepoDirName),
		StageWorkingDir:  filepath.Join(s.workingDir, workspaceStagesDirName, ps.Id),
		CommandLister: stageCommandLister{
			lister:       s.commandLister,
			deploymentID: s.deployment.Id,
			stageID:      ps.Id,
		},
		LogPersister:      lp,
		MetadataStore:     s.metadataStore,
		AppManifestsCache: s.appManifestsCache,
		AppLiveResourceLister: appLiveResourceLister{
			lister:        s.liveResourceLister,
			cloudProvider: app.CloudProvider,
			appID:         app.Id,
		},
		Logger: s.logger,
	}

	// Find the executor for this stage.
	ex, ok := executorFactory(input)
	if !ok {
		err := fmt.Errorf("no registered executor for stage %s", ps.Name)
		lp.Error(err.Error())
		s.reportStageStatus(ctx, ps.Id, model.StageStatus_STAGE_FAILURE, ps.Requires)
		return model.StageStatus_STAGE_FAILURE
	}

	// Start running executor.
	status := ex.Execute(sig)

	if status == model.StageStatus_STAGE_SUCCESS ||
		status == model.StageStatus_STAGE_CANCELLED ||
		(status == model.StageStatus_STAGE_FAILURE && !sig.Stopped()) {

		s.reportStageStatus(ctx, ps.Id, status, ps.Requires)
		return status
	}

	return originalStatus
}

// ensurePreparing ensures that all needed things should be prepared before executing any stages.
// The log of this preparing process will be written to the first executing stage
// when a new scheduler has been created.
//   needTargetCommit = true means the target commit must be prepared
//   needRunningCommit = true means the running commit must be prepared
func (s *scheduler) ensurePreparing(ctx context.Context, needTargetCommit, needRunningCommit bool, lp logpersister.StageLogPersister) error {
	s.prepareMu.Lock()
	defer s.prepareMu.Unlock()

	if s.prepared {
		return nil
	}
	lp.Info("Start preparing repository data for the stage")

	var (
		repoPath = filepath.Join(s.workingDir, workspaceGitRepoDirName)
		appPath  = filepath.Join(repoPath, s.deployment.GitPath.Path)
	)

	// Ensure that this directory is empty.
	// Because it maybe created by another stage before.
	if err := os.RemoveAll(repoPath); err != nil {
		lp.Errorf("Unable to prepare a temporary directory for storing git repository (%v)", err)
		return err
	}

	// Clone repository and checkout to the target revision.
	gitRepo, err := prepareDeployRepository(ctx, s.deployment, s.gitClient, repoPath, s.pipedConfig)
	if err != nil {
		lp.Errorf("Unable to prepare repository (%v)", err)
		return err
	}
	lp.Successf("Successfully cloned repository %s", s.deployment.GitPath.Repo.Id)

	// Load deployment configuration at the target revision.
	cfg, err := loadDeploymentConfiguration(gitRepo.GetPath(), s.deployment)
	if err != nil {
		lp.Errorf("Failed to load deployment configuration (%v)", err)
		return fmt.Errorf("failed to load deployment configuration (%w)", err)
	}
	s.deploymentConfig = cfg

	gds, ok := cfg.GetGenericDeployment()
	if !ok {
		lp.Errorf("Unsupport application kind %s", cfg.Kind)
		return fmt.Errorf("unsupport application kind %s", cfg.Kind)
	}
	s.genericDeploymentSpec = gds
	lp.Success("Successfully loaded deployment configuration")

	if needRunningCommit {
		// Copy and checkout the running revision.
		var (
			runningRepoPath = filepath.Join(s.workingDir, workspaceGitRunningRepoDirName)
			runningAppPath  = filepath.Join(runningRepoPath, s.deployment.GitPath.Path)
		)

		// Ensure that this directory is empty.
		// Because it maybe created by another stage before.
		if err := os.RemoveAll(runningRepoPath); err != nil {
			lp.Errorf("Unable to prepare a temporary directory for storing git repository (%v)", err)
			return err
		}

		runningGitRepo, err := gitRepo.Copy(runningRepoPath)
		if err != nil {
			lp.Errorf("Unable to copy repository (%v)", err)
			return err
		}
		if err = runningGitRepo.Checkout(ctx, s.deployment.RunningCommitHash); err != nil {
			lp.Errorf("Unable to checkout repository (%v)", err)
			return err
		}

		// Load deployment configuration at the running revision.
		cfg, err := loadDeploymentConfiguration(runningRepoPath, s.deployment)
		if err != nil {
			lp.Errorf("Failed to load deployment configuration at the running commit (%v)", err)
			return fmt.Errorf("failed to load deployment configuration at the running commit (%w)", err)
		}
		gds, ok := cfg.GetGenericDeployment()
		if !ok {
			lp.Errorf("Unsupport application kind %s", cfg.Kind)
			return fmt.Errorf("unsupport application kind %s", cfg.Kind)
		}

		// Decrypt the sealed secrets at the running revision.
		if len(gds.SealedSecrets) > 0 && s.sealedSecretDecrypter != nil {
			if err := decryptSealedSecrets(runningAppPath, gds.SealedSecrets, s.sealedSecretDecrypter); err != nil {
				lp.Errorf("Failed to decrypt sealed secrets at running commit (%v)", err)
				return fmt.Errorf("failed to decrypt sealed secrets at running commit (%w)", err)
			}
			lp.Successf("Successsfully decrypted %d sealed secrets at running commit", len(gds.SealedSecrets))
		}
	}

	if needTargetCommit {
		// Decrypt the sealed secrets at the target revision.
		if len(gds.SealedSecrets) > 0 && s.sealedSecretDecrypter != nil {
			if err := decryptSealedSecrets(appPath, gds.SealedSecrets, s.sealedSecretDecrypter); err != nil {
				lp.Errorf("Failed to decrypt sealed secrets (%v)", err)
				return fmt.Errorf("failed to decrypt sealed secrets (%w)", err)
			}
			lp.Successf("Successsfully decrypted %d sealed secrets", len(gds.SealedSecrets))
		}
	}

	s.prepared = true
	lp.Info("All preparations have been completed successfully")

	return nil
}

func (s *scheduler) reportStageStatus(ctx context.Context, stageID string, status model.StageStatus, requires []string) error {
	var (
		err error
		now = s.nowFunc()
		req = &pipedservice.ReportStageStatusChangedRequest{
			DeploymentId: s.deployment.Id,
			StageId:      stageID,
			Status:       status,
			Requires:     requires,
			Visible:      true,
			CompletedAt:  now.Unix(),
		}
		retry = pipedservice.NewRetry(10)
	)

	// Update stage status at local.
	s.stageStatuses[stageID] = status

	// Update stage status on the remote.
	for retry.WaitNext(ctx) {
		_, err = s.apiClient.ReportStageStatusChanged(ctx, req)
		if err == nil {
			break
		}
		err = fmt.Errorf("failed to report stage status to control-plane: %v", err)
	}

	return err
}

func (s *scheduler) reportDeploymentStatusChanged(ctx context.Context, status model.DeploymentStatus, desc string) error {
	var (
		err   error
		retry = pipedservice.NewRetry(10)
		req   = &pipedservice.ReportDeploymentStatusChangedRequest{
			DeploymentId: s.deployment.Id,
			Status:       status,
			StatusReason: desc,
		}
	)

	// Update deployment status on remote.
	for retry.WaitNext(ctx) {
		if _, err = s.apiClient.ReportDeploymentStatusChanged(ctx, req); err == nil {
			return nil
		}
		err = fmt.Errorf("failed to report deployment status to control-plane: %v", err)
	}
	return err
}

func (s *scheduler) reportDeploymentCompleted(ctx context.Context, status model.DeploymentStatus, desc, cancelCommander string) error {
	var (
		err error
		now = s.nowFunc()
		req = &pipedservice.ReportDeploymentCompletedRequest{
			DeploymentId:  s.deployment.Id,
			Status:        status,
			StatusReason:  desc,
			StageStatuses: s.stageStatuses,
			CompletedAt:   now.Unix(),
		}
		retry = pipedservice.NewRetry(10)
	)

	defer func() {
		switch status {
		case model.DeploymentStatus_DEPLOYMENT_SUCCESS:
			s.notifier.Notify(model.Event{
				Type: model.EventType_EVENT_DEPLOYMENT_SUCCEEDED,
				Metadata: &model.EventDeploymentSucceeded{
					Deployment: s.deployment,
					EnvName:    s.envName,
				},
			})

		case model.DeploymentStatus_DEPLOYMENT_FAILURE:
			s.notifier.Notify(model.Event{
				Type: model.EventType_EVENT_DEPLOYMENT_FAILED,
				Metadata: &model.EventDeploymentFailed{
					Deployment: s.deployment,
					EnvName:    s.envName,
					Reason:     desc,
				},
			})

		case model.DeploymentStatus_DEPLOYMENT_CANCELLED:
			s.notifier.Notify(model.Event{
				Type: model.EventType_EVENT_DEPLOYMENT_CANCELLED,
				Metadata: &model.EventDeploymentCancelled{
					Deployment: s.deployment,
					EnvName:    s.envName,
					Commander:  cancelCommander,
				},
			})
		}
	}()

	// Update deployment status on remote.
	for retry.WaitNext(ctx) {
		if _, err = s.apiClient.ReportDeploymentCompleted(ctx, req); err == nil {
			return nil
		}
		err = fmt.Errorf("failed to report deployment status to control-plane: %w", err)
	}

	return err
}

func (s *scheduler) reportMostRecentlySuccessfulDeployment(ctx context.Context) error {
	var (
		err error
		req = &pipedservice.ReportApplicationMostRecentDeploymentRequest{
			ApplicationId: s.deployment.ApplicationId,
			Status:        model.DeploymentStatus_DEPLOYMENT_SUCCESS,
			Deployment: &model.ApplicationDeploymentReference{
				DeploymentId: s.deployment.Id,
				Trigger:      s.deployment.Trigger,
				Summary:      s.deployment.Summary,
				Version:      s.deployment.Version,
				StartedAt:    s.deployment.CreatedAt,
				CompletedAt:  s.deployment.CompletedAt,
			},
		}
		retry = pipedservice.NewRetry(10)
	)

	for retry.WaitNext(ctx) {
		if _, err = s.apiClient.ReportApplicationMostRecentDeployment(ctx, req); err == nil {
			return nil
		}
		err = fmt.Errorf("failed to report most recent successful deployment: %w", err)
	}
	return err
}

type stageCommandLister struct {
	lister       commandLister
	deploymentID string
	stageID      string
}

func (s stageCommandLister) ListCommands() []model.ReportableCommand {
	return s.lister.ListStageCommands(s.deploymentID, s.stageID)
}
