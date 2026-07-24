package swarm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestUpdateTeamConfigConcurrentMembersHasNoLostWrites(t *testing.T) {
	withTempHome(t, func(_ string) {
		if err := CreateTeamConfigAs("update-team", &TeamConfig{Name: "update-team", LeadAgentID: "lead"}); err != nil {
			t.Fatal(err)
		}
		const count = 100
		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make(chan error, count)
		for index := range count {
			index := index
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := UpdateTeamConfig(context.Background(), "update-team", func(config *TeamConfig) error {
					id := fmt.Sprintf("agent-%03d", index)
					config.Members = append(config.Members, TeamMember{AgentID: id, Name: id, IsActive: true})
					return nil
				})
				errs <- err
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		config, err := LoadTeamConfig("update-team")
		if err != nil {
			t.Fatal(err)
		}
		if len(config.Members) != count || config.Revision != count+1 {
			t.Fatalf("members=%d revision=%d", len(config.Members), config.Revision)
		}
		seen := make(map[string]struct{}, count)
		for _, member := range config.Members {
			seen[member.AgentID] = struct{}{}
		}
		if len(seen) != count {
			t.Fatalf("unique members=%d", len(seen))
		}
	})
}

func TestCreateTeamConfigAsHasSingleConcurrentWinner(t *testing.T) {
	withTempHome(t, func(_ string) {
		const count = 32
		start := make(chan struct{})
		var wg sync.WaitGroup
		var winners atomic.Int32
		errs := make(chan error, count)
		for index := range count {
			index := index
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				err := CreateTeamConfigAs("exclusive", &TeamConfig{
					Name: "exclusive", LeadAgentID: fmt.Sprintf("lead-%d", index),
				})
				if err == nil {
					winners.Add(1)
				}
				errs <- err
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil && !errors.Is(err, ErrTeamConfigExists) {
				t.Fatalf("unexpected create error: %v", err)
			}
		}
		if winners.Load() != 1 {
			t.Fatalf("winners=%d", winners.Load())
		}
		config, err := LoadTeamConfig("exclusive")
		if err != nil || config.LeadAgentID == "" || config.Revision != 1 {
			t.Fatalf("winner config=%#v err=%v", config, err)
		}
	})
}

func TestDeleteTeamConfigAndConcurrentUpdateCannotResurrectConfig(t *testing.T) {
	withTempHome(t, func(_ string) {
		if err := CreateTeamConfigAs("delete-race", &TeamConfig{Name: "delete-race", LeadAgentID: "lead"}); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		errs := make(chan error, 2)
		go func() {
			defer wg.Done()
			<-start
			_, err := UpdateTeamConfig(context.Background(), "delete-race", func(config *TeamConfig) error {
				config.Description = "updated"
				return nil
			})
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				// The wrapped read error is expected when deletion wins the lock.
				if !strings.Contains(err.Error(), "no such file") {
					errs <- err
					return
				}
			}
			errs <- nil
		}()
		go func() {
			defer wg.Done()
			<-start
			errs <- DeleteTeamConfig("delete-race")
		}()
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if _, err := LoadTeamConfig("delete-race"); err == nil {
			t.Fatal("concurrent update resurrected deleted config")
		}
	})
}
