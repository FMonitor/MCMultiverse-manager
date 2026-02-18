package cronjob

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mcmm/internal/log"
	"mcmm/internal/pgsql"
	"mcmm/internal/servertap"
	"mcmm/internal/worker"
)

var playersRegex = regexp.MustCompile(`(?i)there are\s+(\d+)\s+out of`)

type Scheduler struct {
	repos pgsql.Repos
	w     worker.Worker
	opts  Options
	log   interface {
		Infof(string, ...any)
		Warnf(string, ...any)
		Errorf(string, ...any)
	}
}

type Options struct {
	OffInterval       time.Duration
	RemoveDays        int
	InstanceTapURLFmt string
	ServerTapTimeout  time.Duration
	ServerTapAuthName string
	ServerTapAuthKey  string
	Now               func() time.Time
}

func NewScheduler(repos pgsql.Repos, w worker.Worker, opts Options) *Scheduler {
	if opts.OffInterval <= 0 {
		opts.OffInterval = time.Hour
	}
	if opts.RemoveDays <= 0 {
		opts.RemoveDays = 14
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Scheduler{
		repos: repos,
		w:     w,
		opts:  opts,
		log:   log.Component("cronjob"),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	go s.runIdleLoop(ctx)
	go s.runArchiveLoop(ctx)
}

func (s *Scheduler) runIdleLoop(ctx context.Context) {
	tk := time.NewTicker(s.opts.OffInterval)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			s.runIdleOnce(ctx)
		}
	}
}

func (s *Scheduler) runArchiveLoop(ctx context.Context) {
	tk := time.NewTicker(24 * time.Hour)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			s.runArchiveOnce(ctx)
		}
	}
}

func (s *Scheduler) runIdleOnce(ctx context.Context) {
	list, err := s.repos.MapInstance.List(ctx)
	if err != nil {
		s.log.Warnf("idle check list instances failed: %v", err)
		return
	}
	for _, inst := range list {
		if inst.Status != string(worker.StatusOn) {
			continue
		}
		hasPlayers, known, err := s.instanceHasPlayers(ctx, inst.ID)
		if err != nil {
			s.log.Warnf("idle check instance=%d failed: %v", inst.ID, err)
			continue
		}
		if !known {
			s.log.Infof("idle check instance=%d skipped (player count unavailable)", inst.ID)
			continue
		}
		if hasPlayers {
			continue
		}
		s.log.Infof("idle auto-off instance=%d alias=%s", inst.ID, inst.Alias)
		if err := s.w.StopOnly(context.Background(), inst.ID); err != nil {
			s.log.Errorf("idle auto-off instance=%d failed: %v", inst.ID, err)
		}
	}
}

func (s *Scheduler) runArchiveOnce(ctx context.Context) {
	list, err := s.repos.MapInstance.List(ctx)
	if err != nil {
		s.log.Warnf("archive check list instances failed: %v", err)
		return
	}
	cutoff := s.opts.Now().AddDate(0, 0, -s.opts.RemoveDays)
	for _, inst := range list {
		if inst.Status != string(worker.StatusOff) {
			continue
		}
		last := inst.UpdatedAt
		if inst.LastActiveAt.Valid {
			last = inst.LastActiveAt.Time
		}
		if last.After(cutoff) {
			continue
		}
		s.log.Infof("auto-archive instance=%d alias=%s last=%s cutoff=%s", inst.ID, inst.Alias, last.Format(time.RFC3339), cutoff.Format(time.RFC3339))
		if err := s.w.StopAndArchive(context.Background(), inst.ID); err != nil {
			s.log.Errorf("auto-archive instance=%d failed: %v", inst.ID, err)
		}
	}
}

func (s *Scheduler) instanceHasPlayers(ctx context.Context, instanceID int64) (hasPlayers bool, known bool, err error) {
	if strings.TrimSpace(s.opts.InstanceTapURLFmt) == "" {
		return false, false, nil
	}
	url := fmt.Sprintf(strings.TrimSpace(s.opts.InstanceTapURLFmt), instanceID)
	conn, err := servertap.NewConnectorWithAuth(url, s.opts.ServerTapTimeout, s.opts.ServerTapAuthName, s.opts.ServerTapAuthKey)
	if err != nil {
		return false, false, err
	}
	resp, err := conn.Execute(ctx, servertap.ExecuteRequest{Command: "list"})
	if err != nil {
		return false, false, err
	}
	body := strings.TrimSpace(resp.RawBody)
	if body == "" {
		return false, false, nil
	}
	m := playersRegex.FindStringSubmatch(body)
	if len(m) != 2 {
		return false, false, nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return false, false, nil
	}
	return n > 0, true, nil
}
