package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bricef/taskflow/internal/model"
)

const maxDetailTasks = 500

// BoardDetail returns a complete board snapshot with all tasks and their
// nested comments, attachments, dependencies, and audit entries.
func (s *Service) BoardDetail(ctx context.Context, slug string) (model.BoardDetail, error) {
	board, err := s.store.BoardGet(ctx, slug)
	if err != nil {
		return model.BoardDetail{}, err
	}

	wf, err := s.GetWorkflow(ctx, slug)
	if err != nil {
		return model.BoardDetail{}, err
	}

	tasks, err := s.store.TaskList(ctx, model.TaskFilter{
		BoardSlug:      slug,
		IncludeClosed:  true,
		IncludeDeleted: false,
	}, nil)
	if err != nil {
		return model.BoardDetail{}, err
	}

	boardAudit, err := s.store.AuditQueryByBoard(ctx, slug)
	if err != nil {
		return model.BoardDetail{}, err
	}

	if len(tasks) > maxDetailTasks {
		tasks = tasks[:maxDetailTasks]
	}

	taskDetails := make([]model.TaskDetail, 0, len(tasks))
	for _, t := range tasks {
		comments, _ := s.store.CommentList(ctx, slug, t.Num)
		attachments, _ := s.store.AttachmentList(ctx, slug, t.Num)
		deps, _ := s.store.DependencyList(ctx, slug, t.Num)
		audit, _ := s.store.AuditQueryByTask(ctx, slug, t.Num)

		taskDetails = append(taskDetails, model.TaskDetail{
			Task:         t,
			Comments:     orEmpty(comments),
			Attachments:  orEmpty(attachments),
			Dependencies: orEmpty(deps),
			Audit:        orEmpty(audit),
		})
	}

	return model.BoardDetail{
		Board:    board,
		Workflow: wf,
		Tasks:    taskDetails,
		Audit:    orEmpty(boardAudit),
	}, nil
}

// BoardOverview returns board metadata with task counts by state.
func (s *Service) BoardOverview(ctx context.Context, slug string) (model.BoardOverview, error) {
	board, err := s.store.BoardGet(ctx, slug)
	if err != nil {
		return model.BoardOverview{}, err
	}

	tasks, err := s.store.TaskList(ctx, model.TaskFilter{
		BoardSlug:     slug,
		IncludeClosed: true,
	}, nil)
	if err != nil {
		return model.BoardOverview{}, err
	}

	counts := map[string]int{}
	for _, t := range tasks {
		counts[t.State]++
	}

	return model.BoardOverview{
		Board:      board,
		TaskCounts: counts,
		TotalTasks: len(tasks),
	}, nil
}

// SystemStats returns system-wide statistics across all actors, boards,
// tasks, and audit activity.
func (s *Service) SystemStats(ctx context.Context) (model.SystemStats, error) {
	actors, err := s.store.ActorList(ctx)
	if err != nil {
		return model.SystemStats{}, err
	}
	as := model.ActorStats{ByRole: map[string]int{}}
	for _, a := range actors {
		as.Total++
		if a.Active {
			as.Active++
		}
		as.ByRole[string(a.Role)]++
	}

	allBoards, err := s.store.BoardList(ctx, model.ListBoardsParams{IncludeDeleted: true})
	if err != nil {
		return model.SystemStats{}, err
	}
	bs := model.BoardStats{Total: len(allBoards)}
	for _, b := range allBoards {
		if !b.Deleted {
			bs.Active++
		}
	}

	ts := model.TaskStatsSummary{ByState: map[string]int{}}
	sevenDaysAgo := time.Now().UTC().AddDate(0, 0, -7)

	totalEvents := 0
	last7dEvents := 0
	actorEvents := map[string]int{}

	for _, b := range allBoards {
		if b.Deleted {
			continue
		}
		tasks, err := s.store.TaskList(ctx, model.TaskFilter{
			BoardSlug:      b.Slug,
			IncludeClosed:  true,
			IncludeDeleted: false,
		}, nil)
		if err != nil {
			continue
		}
		for _, t := range tasks {
			ts.Total++
			ts.ByState[t.State]++
			if t.CreatedAt.After(sevenDaysAgo) {
				ts.CreatedLast7d++
			}
		}

		audit, err := s.store.AuditQueryByBoard(ctx, b.Slug)
		if err != nil {
			continue
		}
		totalEvents += len(audit)
		for _, e := range audit {
			if e.Action == model.AuditActionTransitioned && e.CreatedAt.After(sevenDaysAgo) {
				var detail map[string]any
				json.Unmarshal(e.Detail, &detail)
				if to, ok := detail["to"].(string); ok {
					if to == "done" || to == "cancelled" {
						ts.CompletedLast7d++
					}
				}
			}
			if e.CreatedAt.After(sevenDaysAgo) {
				last7dEvents++
				actorEvents[e.Actor]++
			}
		}
	}

	var byActor []model.ActorActivity
	for name, count := range actorEvents {
		byActor = append(byActor, model.ActorActivity{Name: name, EventsLast7d: count})
	}

	return model.SystemStats{
		Actors: as,
		Boards: bs,
		Tasks:  ts,
		Activity: model.ActivityStats{
			TotalEvents: totalEvents,
			Last7d:      last7dEvents,
			ByActor:     orEmpty(byActor),
		},
	}, nil
}

// orEmpty ensures nil slices serialize as [] rather than null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
