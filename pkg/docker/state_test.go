package docker

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBackupDue(t *testing.T) {
	t.Run("due when app has no state", func(t *testing.T) {
		s := &State{}
		assert.True(t, s.BackupDue("myapp"))
	})

	t.Run("due when no last backup", func(t *testing.T) {
		s := &State{Apps: map[string]*AppState{"myapp": {}}}
		assert.True(t, s.BackupDue("myapp"))
	})

	t.Run("due when last backup exceeds interval", func(t *testing.T) {
		s := &State{Apps: map[string]*AppState{
			"myapp": {LastBackup: OperationResult{At: time.Now().Add(-AutomaticTaskInterval - time.Minute)}},
		}}
		assert.True(t, s.BackupDue("myapp"))
	})

	t.Run("not due when last backup was recent and successful", func(t *testing.T) {
		s := &State{Apps: map[string]*AppState{
			"myapp": {LastBackup: OperationResult{At: time.Now().Add(-time.Second)}},
		}}
		assert.False(t, s.BackupDue("myapp"))
	})

	t.Run("due when last backup was recent but had an error", func(t *testing.T) {
		s := &State{Apps: map[string]*AppState{
			"myapp": {LastBackup: OperationResult{At: time.Now().Add(-time.Second), Error: "disk full"}},
		}}
		assert.True(t, s.BackupDue("myapp"))
	})
}

func TestUpdateDue(t *testing.T) {
	t.Run("due when app has no state", func(t *testing.T) {
		s := &State{}
		assert.True(t, s.UpdateDue("myapp"))
	})

	t.Run("not due when last update was recent and successful", func(t *testing.T) {
		s := &State{Apps: map[string]*AppState{
			"myapp": {LastUpdate: OperationResult{At: time.Now().Add(-time.Second)}},
		}}
		assert.False(t, s.UpdateDue("myapp"))
	})

	t.Run("due when last update was recent but had an error", func(t *testing.T) {
		s := &State{Apps: map[string]*AppState{
			"myapp": {LastUpdate: OperationResult{At: time.Now().Add(-time.Second), Error: "pull failed"}},
		}}
		assert.True(t, s.UpdateDue("myapp"))
	})
}

func TestRecordBackup(t *testing.T) {
	t.Run("records successful backup", func(t *testing.T) {
		s := &State{}
		s.RecordBackup("myapp", nil)

		assert.False(t, s.Apps["myapp"].LastBackup.At.IsZero())
		assert.Empty(t, s.Apps["myapp"].LastBackup.Error)
		assert.WithinDuration(t, time.Now(), s.Apps["myapp"].LastBackup.At, time.Second)
	})

	t.Run("records failed backup", func(t *testing.T) {
		s := &State{}
		s.RecordBackup("myapp", errors.New("disk full"))

		assert.Equal(t, "disk full", s.Apps["myapp"].LastBackup.Error)
		assert.WithinDuration(t, time.Now(), s.Apps["myapp"].LastBackup.At, time.Second)
	})
}

func TestRecordUpdate(t *testing.T) {
	t.Run("records successful update", func(t *testing.T) {
		s := &State{}
		s.RecordUpdate("myapp", nil)

		assert.False(t, s.Apps["myapp"].LastUpdate.At.IsZero())
		assert.Empty(t, s.Apps["myapp"].LastUpdate.Error)
	})

	t.Run("records failed update", func(t *testing.T) {
		s := &State{}
		s.RecordUpdate("myapp", errors.New("pull failed"))

		assert.Equal(t, "pull failed", s.Apps["myapp"].LastUpdate.Error)
	})
}
