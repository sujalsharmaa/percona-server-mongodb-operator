package perconaservermongodbclustersync

import (
	api "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
)

type modeAction string

const (
	actionNone     modeAction = ""
	actionStart    modeAction = "start"
	actionResume   modeAction = "resume"
	actionPause    modeAction = "pause"
	actionReset    modeAction = "reset"
	actionFinalize modeAction = "finalize"
)

func nextAction(statusMode, specMode api.ClusterSyncMode, hasStarted bool) (modeAction, bool) {
	if statusMode == api.ClusterSyncModeFinalized {
		return actionNone, false
	}
	if statusMode == specMode {
		return actionNone, false
	}

	from := statusMode
	if from == "" {
		from = api.ClusterSyncModePaused
	}

	switch specMode {
	case api.ClusterSyncModePaused:
		switch from {
		case api.ClusterSyncModePaused:
			return actionNone, true
		case api.ClusterSyncModeRunning:
			return actionPause, true
		case api.ClusterSyncModeReset:
			return actionNone, true
		}
	case api.ClusterSyncModeRunning:
		switch from {
		case api.ClusterSyncModePaused:
			if hasStarted {
				return actionResume, true
			}
			return actionStart, true
		case api.ClusterSyncModeReset:
			return actionStart, true
		}
	case api.ClusterSyncModeReset:
		switch from {
		case api.ClusterSyncModePaused, api.ClusterSyncModeRunning:
			return actionReset, true
		}
	case api.ClusterSyncModeFinalized:
		switch from {
		case api.ClusterSyncModeRunning:
			return actionFinalize, true
		case api.ClusterSyncModePaused:
			if hasStarted {
				return actionFinalize, true
			}
		}
	}
	return actionNone, false
}
