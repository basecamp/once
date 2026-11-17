package background

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/userstats"
	"github.com/basecamp/once/internal/version"
)

const CheckInterval = 5 * time.Minute

type Runner struct {
	namespace string

	// Set once a proxy replacement has failed after stopping the old proxy.
	// Retrying on every check would take the apps offline every few minutes,
	// so the runner waits until it's next restarted (by a self-update, or the
	// host rebooting) to try again.
	proxyUpgradeRolledBack bool
}

func NewRunner(namespace string) *Runner {
	return &Runner{
		namespace: namespace,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	slog.Info("Starting background runner", "namespace", r.namespace, "check_interval", CheckInterval)

	scraper := userstats.NewScraper(r.namespace)
	go scraper.Run(ctx)

	r.upgradeProxy(ctx)

	ticker := time.NewTicker(CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Shutting down")
			return nil
		case <-ticker.C:
			if updated, err := r.check(ctx); err != nil {
				slog.Error("Check failed", "error", err)
			} else if updated {
				return nil
			}
		}
	}
}

// Private

func (r *Runner) check(ctx context.Context) (bool, error) {
	ns, err := docker.RestoreNamespace(ctx, r.namespace)
	if err != nil {
		return false, fmt.Errorf("restoring namespace: %w", err)
	}

	state, err := ns.LoadState(ctx)
	if err != nil {
		return false, fmt.Errorf("loading state: %w", err)
	}

	if r.checkSelfUpdate(ctx, ns, state) {
		return true, nil
	}

	r.upgradeProxyIn(ctx, ns)

	for _, app := range ns.Applications() {
		if !app.Running {
			continue
		}

		r.checkUpdate(ctx, app, state)
		r.checkBackup(ctx, app, state)
	}

	return false, nil
}

// upgradeProxy runs at startup, so a proxy replacement interrupted by the
// runner being stopped is recovered straight away rather than at the first
// check.
func (r *Runner) upgradeProxy(ctx context.Context) {
	ns, err := docker.NewNamespace(r.namespace)
	if err != nil {
		slog.Error("Proxy upgrade failed", "error", err)
		return
	}
	r.upgradeProxyIn(ctx, ns)
}

func (r *Runner) upgradeProxyIn(ctx context.Context, ns *docker.Namespace) {
	if r.proxyUpgradeRolledBack {
		return
	}

	err := ns.Proxy().Upgrade(ctx)
	if err != nil {
		slog.Error("Proxy upgrade failed", "error", err)
	}
	if errors.Is(err, docker.ErrProxyReplaceRolledBack) {
		slog.Warn("Not retrying the proxy upgrade until the background runner restarts")
		r.proxyUpgradeRolledBack = true
	}
}

func (r *Runner) checkSelfUpdate(ctx context.Context, ns *docker.Namespace, state *docker.State) bool {
	if os.Getenv("ONCE_NO_SELF_UPDATE") != "" {
		return false
	}

	if !state.SelfUpdateDue() {
		return false
	}

	slog.Info("Checking for once update")

	err := version.NewUpdater().UpdateBinary()
	state.RecordSelfUpdate(err)
	if saveErr := ns.SaveState(ctx, state); saveErr != nil {
		slog.Error("Failed to save state after self-update check", "error", saveErr)
	}

	if err != nil {
		slog.Error("Self-update failed", "error", err)
		return false
	}

	slog.Info("Self-update complete, restarting")
	return true
}

func (r *Runner) checkUpdate(ctx context.Context, app *docker.Application, state *docker.State) {
	if !app.Settings.AutoUpdate {
		return
	}
	if !state.UpdateDue(app.Settings.Name) {
		return
	}

	slog.Info("Running auto-update", "app", app.Settings.Name)

	changed, err := app.Update(ctx, nil)
	if err != nil {
		slog.Error("Auto-update failed", "app", app.Settings.Name, "error", err)
	} else if changed {
		slog.Info("Auto-update completed", "app", app.Settings.Name)
	} else {
		slog.Info("Already up to date", "app", app.Settings.Name)
	}
}

func (r *Runner) checkBackup(ctx context.Context, app *docker.Application, state *docker.State) {
	if !app.Settings.Backup.AutoBackup {
		return
	}
	if !state.BackupDue(app.Settings.Name) {
		return
	}

	slog.Info("Running auto-backup", "app", app.Settings.Name)

	if err := app.Backup(ctx); err != nil {
		slog.Error("Auto-backup failed", "app", app.Settings.Name, "error", err)
	} else {
		slog.Info("Auto-backup completed", "app", app.Settings.Name)
	}

	if err := app.TrimBackups(); err != nil {
		slog.Error("Backup trim failed", "app", app.Settings.Name, "error", err)
	}
}
