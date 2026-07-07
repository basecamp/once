package docker

import "time"

type State struct {
	Apps           map[string]*AppState `json:"apps"`
	LastSelfUpdate OperationResult      `json:"lastSelfUpdate"`
}

type AppState struct {
	LastBackup OperationResult `json:"lastBackup"`
	LastUpdate OperationResult `json:"lastUpdate"`
}

func (as *AppState) LastBackupResult() *OperationResult {
	if as == nil || as.LastBackup.At.IsZero() {
		return nil
	}
	return &as.LastBackup
}

func (as *AppState) LastUpdateResult() *OperationResult {
	if as == nil || as.LastUpdate.At.IsZero() {
		return nil
	}
	return &as.LastUpdate
}

type OperationResult struct {
	At    time.Time `json:"at"`
	Error string    `json:"error"`
}

func (s *State) BackupDue(appName string) bool {
	return s.operationDue(appName, func(as *AppState) OperationResult { return as.LastBackup })
}

func (s *State) UpdateDue(appName string) bool {
	return s.operationDue(appName, func(as *AppState) OperationResult { return as.LastUpdate })
}

func (s *State) AppState(appName string) *AppState {
	if s.Apps == nil {
		return nil
	}
	return s.Apps[appName]
}

func (s *State) SelfUpdateDue() bool {
	if s.LastSelfUpdate.At.IsZero() {
		return true
	}
	return s.LastSelfUpdate.Error != "" || time.Since(s.LastSelfUpdate.At) >= AutomaticTaskInterval
}

func (s *State) RecordSelfUpdate(err error) {
	s.LastSelfUpdate = newResult(err)
}

func (s *State) RecordBackup(appName string, err error) {
	s.ensureApp(appName).LastBackup = newResult(err)
}

func (s *State) RecordUpdate(appName string, err error) {
	s.ensureApp(appName).LastUpdate = newResult(err)
}

// Private

func (s *State) operationDue(appName string, getResult func(*AppState) OperationResult) bool {
	app, ok := s.Apps[appName]
	if !ok {
		return true
	}

	result := getResult(app)
	if result.At.IsZero() {
		return true
	}

	return result.Error != "" || time.Since(result.At) >= AutomaticTaskInterval
}

func (s *State) ensureApp(appName string) *AppState {
	if s.Apps == nil {
		s.Apps = make(map[string]*AppState)
	}

	app, ok := s.Apps[appName]
	if !ok {
		app = &AppState{}
		s.Apps[appName] = app
	}

	return app
}

// Helpers

func newResult(err error) OperationResult {
	r := OperationResult{At: time.Now()}
	if err != nil {
		r.Error = ErrorMessage(err)
	}
	return r
}
