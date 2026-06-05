package perconaservermongodbclustersync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/percona/percona-server-mongodb-operator/pkg/psmdb/clustersync/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
)

func TestApplyObservedStatus(t *testing.T) {
	tests := map[string]struct {
		initial                psmdbv1.PerconaServerMongoDBClusterSyncStatus
		observed               client.Status
		wantState              psmdbv1.ClusterSyncState
		wantLag                int64
		wantError              string
		wantStartedAtSet       bool
		wantStartedAtUnchanged bool
		wantConditions         []string
	}{
		"pending does not set startedAt": {
			observed:  client.Status{State: "pending"},
			wantState: psmdbv1.ClusterSyncStatePending,
		},
		"first non-pending state sets startedAt": {
			observed:         client.Status{State: "initialSync"},
			wantState:        psmdbv1.ClusterSyncStateInitialSync,
			wantStartedAtSet: true,
		},
		"existing startedAt is preserved across restarts": {
			initial: psmdbv1.PerconaServerMongoDBClusterSyncStatus{
				StartedAt: &metav1.Time{Time: time.Now().Add(-time.Hour)},
			},
			observed:               client.Status{State: "pending"},
			wantState:              psmdbv1.ClusterSyncStatePending,
			wantStartedAtUnchanged: true,
		},
		"replicating emits InitialSyncComplete + Replicating conditions": {
			observed:         client.Status{State: "replicating", LagTimeSeconds: 2},
			wantState:        psmdbv1.ClusterSyncStateReplicating,
			wantLag:          2,
			wantStartedAtSet: true,
			wantConditions: []string{
				psmdbv1.ConditionClusterSyncInitialSyncComplete,
				psmdbv1.ConditionClusterSyncReplicating,
			},
		},
		"finalized emits Finalized condition": {
			observed:         client.Status{State: "finalized"},
			wantState:        psmdbv1.ClusterSyncStateFinalized,
			wantStartedAtSet: true,
			wantConditions:   []string{psmdbv1.ConditionClusterSyncFinalized},
		},
		"failed propagates error message": {
			observed:         client.Status{State: "failed", Error: "source unreachable"},
			wantState:        psmdbv1.ClusterSyncStateFailed,
			wantError:        "source unreachable",
			wantStartedAtSet: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := tc.initial.DeepCopy()
			before := s.StartedAt

			applyObservedStatus(s, tc.observed)

			assert.Equal(t, tc.wantState, s.State)
			assert.Equal(t, tc.wantLag, s.LagTimeSeconds)
			assert.Equal(t, tc.wantError, s.Error)

			switch {
			case tc.wantStartedAtUnchanged:
				assert.Equal(t, before, s.StartedAt)
			case tc.wantStartedAtSet:
				require.NotNil(t, s.StartedAt)
			default:
				assert.Nil(t, s.StartedAt)
			}

			for _, ct := range tc.wantConditions {
				assert.True(t, meta.IsStatusConditionTrue(s.Conditions, ct), "expected condition %s=True", ct)
			}
		})
	}
}

// fakePCSM records every call so InvokeAction tests can assert exactly
// one verb fires per action with the expected arguments.
type fakePCSM struct {
	started    *client.StartOptions
	paused     int
	resumed    *bool
	reset      int
	finalized  int
	statusResp client.Status
	statusErr  error
	actionErr  error
}

func (f *fakePCSM) Status(_ context.Context) (client.Status, error) {
	return f.statusResp, f.statusErr
}
func (f *fakePCSM) Start(_ context.Context, opts client.StartOptions) error {
	f.started = &opts
	return f.actionErr
}
func (f *fakePCSM) Pause(_ context.Context) error {
	f.paused++
	return f.actionErr
}
func (f *fakePCSM) Resume(_ context.Context, fromFailure bool) error {
	f.resumed = &fromFailure
	return f.actionErr
}
func (f *fakePCSM) Reset(_ context.Context) error {
	f.reset++
	return f.actionErr
}
func (f *fakePCSM) Finalize(_ context.Context) error {
	f.finalized++
	return f.actionErr
}

func TestInvokeAction(t *testing.T) {
	tests := map[string]struct {
		action  modeAction
		cr      *psmdbv1.PerconaServerMongoDBClusterSync
		assert  func(t *testing.T, f *fakePCSM)
		wantErr error
	}{
		"start passes excludeNamespaces from spec": {
			action: actionStart,
			cr: &psmdbv1.PerconaServerMongoDBClusterSync{
				Spec: psmdbv1.PerconaServerMongoDBClusterSyncSpec{
					ExcludeNamespaces: []string{"db.*", "tmp.coll"},
				},
			},
			assert: func(t *testing.T, f *fakePCSM) {
				require.NotNil(t, f.started)
				assert.Equal(t, []string{"db.*", "tmp.coll"}, f.started.ExcludeNamespaces)
			},
		},
		"resume passes fromFailure=true when last state was failed": {
			action: actionResume,
			cr: &psmdbv1.PerconaServerMongoDBClusterSync{
				Status: psmdbv1.PerconaServerMongoDBClusterSyncStatus{
					State: psmdbv1.ClusterSyncStateFailed,
				},
			},
			assert: func(t *testing.T, f *fakePCSM) {
				require.NotNil(t, f.resumed)
				assert.True(t, *f.resumed)
			},
		},
		"resume passes fromFailure=false from clean paused": {
			action: actionResume,
			cr: &psmdbv1.PerconaServerMongoDBClusterSync{
				Status: psmdbv1.PerconaServerMongoDBClusterSyncStatus{
					State: psmdbv1.ClusterSyncStatePaused,
				},
			},
			assert: func(t *testing.T, f *fakePCSM) {
				require.NotNil(t, f.resumed)
				assert.False(t, *f.resumed)
			},
		},
		"pause calls /pause": {
			action: actionPause,
			cr:     &psmdbv1.PerconaServerMongoDBClusterSync{},
			assert: func(t *testing.T, f *fakePCSM) { assert.Equal(t, 1, f.paused) },
		},
		"reset calls /reset": {
			action: actionReset,
			cr:     &psmdbv1.PerconaServerMongoDBClusterSync{},
			assert: func(t *testing.T, f *fakePCSM) { assert.Equal(t, 1, f.reset) },
		},
		"finalize calls /finalize": {
			action: actionFinalize,
			cr:     &psmdbv1.PerconaServerMongoDBClusterSync{},
			assert: func(t *testing.T, f *fakePCSM) { assert.Equal(t, 1, f.finalized) },
		},
		"none is a noop": {
			action: actionNone,
			cr:     &psmdbv1.PerconaServerMongoDBClusterSync{},
			assert: func(t *testing.T, f *fakePCSM) {
				assert.Nil(t, f.started)
				assert.Nil(t, f.resumed)
				assert.Zero(t, f.paused+f.reset+f.finalized)
			},
		},
		"propagates client error": {
			action:  actionPause,
			cr:      &psmdbv1.PerconaServerMongoDBClusterSync{},
			wantErr: errors.New("some error"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &fakePCSM{}
			if tc.wantErr != nil {
				f.actionErr = tc.wantErr
			}
			err := invokeAction(t.Context(), f, tc.action, tc.cr)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.assert != nil {
				tc.assert(t, f)
			}
		})
	}
}
