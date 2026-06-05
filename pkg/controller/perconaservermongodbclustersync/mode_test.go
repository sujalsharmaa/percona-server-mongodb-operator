package perconaservermongodbclustersync

import (
	"testing"

	"github.com/stretchr/testify/assert"

	api "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
)

func TestNextAction(t *testing.T) {
	tests := map[string]struct {
		statusMode api.ClusterSyncMode
		specMode   api.ClusterSyncMode
		hasStarted bool
		wantAction modeAction
		wantMirror bool
	}{
		"empty to paused mirrors without HTTP call": {
			statusMode: "",
			specMode:   api.ClusterSyncModePaused,
			wantAction: actionNone,
			wantMirror: true,
		},
		"empty to running issues /start (never started)": {
			statusMode: "",
			specMode:   api.ClusterSyncModeRunning,
			wantAction: actionStart,
			wantMirror: true,
		},
		"empty to reset issues /reset": {
			statusMode: "",
			specMode:   api.ClusterSyncModeReset,
			wantAction: actionReset,
			wantMirror: true,
		},
		"empty to finalized is rejected": {
			statusMode: "",
			specMode:   api.ClusterSyncModeFinalized,
			wantAction: actionNone,
			wantMirror: false,
		},

		"paused == paused noop": {
			statusMode: api.ClusterSyncModePaused,
			specMode:   api.ClusterSyncModePaused,
			wantAction: actionNone,
			wantMirror: false,
		},
		"running == running noop": {
			statusMode: api.ClusterSyncModeRunning,
			specMode:   api.ClusterSyncModeRunning,
			wantAction: actionNone,
			wantMirror: false,
		},

		"paused to running on first start": {
			statusMode: api.ClusterSyncModePaused,
			specMode:   api.ClusterSyncModeRunning,
			hasStarted: false,
			wantAction: actionStart,
			wantMirror: true,
		},
		"paused to running after prior run resumes": {
			statusMode: api.ClusterSyncModePaused,
			specMode:   api.ClusterSyncModeRunning,
			hasStarted: true,
			wantAction: actionResume,
			wantMirror: true,
		},

		"running to paused issues /pause": {
			statusMode: api.ClusterSyncModeRunning,
			specMode:   api.ClusterSyncModePaused,
			wantAction: actionPause,
			wantMirror: true,
		},

		"running to reset issues /reset": {
			statusMode: api.ClusterSyncModeRunning,
			specMode:   api.ClusterSyncModeReset,
			wantAction: actionReset,
			wantMirror: true,
		},
		"paused to reset issues /reset": {
			statusMode: api.ClusterSyncModePaused,
			specMode:   api.ClusterSyncModeReset,
			wantAction: actionReset,
			wantMirror: true,
		},

		"reset to running issues /start": {
			statusMode: api.ClusterSyncModeReset,
			specMode:   api.ClusterSyncModeRunning,
			hasStarted: true,
			wantAction: actionStart,
			wantMirror: true,
		},
		"reset to paused mirrors without HTTP call": {
			statusMode: api.ClusterSyncModeReset,
			specMode:   api.ClusterSyncModePaused,
			wantAction: actionNone,
			wantMirror: true,
		},

		"running to finalized issues /finalize": {
			statusMode: api.ClusterSyncModeRunning,
			specMode:   api.ClusterSyncModeFinalized,
			wantAction: actionFinalize,
			wantMirror: true,
		},
		"paused to finalized issues /finalize when previously started": {
			statusMode: api.ClusterSyncModePaused,
			specMode:   api.ClusterSyncModeFinalized,
			hasStarted: true,
			wantAction: actionFinalize,
			wantMirror: true,
		},
		"paused to finalized rejected when never started": {
			statusMode: api.ClusterSyncModePaused,
			specMode:   api.ClusterSyncModeFinalized,
			hasStarted: false,
			wantAction: actionNone,
			wantMirror: false,
		},
		"reset to finalized rejected": {
			statusMode: api.ClusterSyncModeReset,
			specMode:   api.ClusterSyncModeFinalized,
			wantAction: actionNone,
			wantMirror: false,
		},

		"finalized to running rejected": {
			statusMode: api.ClusterSyncModeFinalized,
			specMode:   api.ClusterSyncModeRunning,
			wantAction: actionNone,
			wantMirror: false,
		},
		"finalized to paused rejected": {
			statusMode: api.ClusterSyncModeFinalized,
			specMode:   api.ClusterSyncModePaused,
			wantAction: actionNone,
			wantMirror: false,
		},
		"finalized to reset rejected": {
			statusMode: api.ClusterSyncModeFinalized,
			specMode:   api.ClusterSyncModeReset,
			wantAction: actionNone,
			wantMirror: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotAction, gotMirror := nextAction(tc.statusMode, tc.specMode, tc.hasStarted)
			assert.Equal(t, tc.wantAction, gotAction, "action")
			assert.Equal(t, tc.wantMirror, gotMirror, "mirror")
		})
	}
}
