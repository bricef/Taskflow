package http

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/bricef/taskflow/internal/model"
)

type systemStats struct {
	Actors   actorStats    `json:"actors"`
	Boards   boardStats    `json:"boards"`
	Tasks    taskStats     `json:"tasks"`
	Activity activityStats `json:"activity"`
}

type actorStats struct {
	Total  int            `json:"total"`
	Active int            `json:"active"`
	ByRole map[string]int `json:"by_role"`
}

type boardStats struct {
	Total  int `json:"total"`
	Active int `json:"active"`
}

type taskStats struct {
	Total           int            `json:"total"`
	ByState         map[string]int `json:"by_state"`
	CreatedLast7d   int            `json:"created_last_7d"`
	CompletedLast7d int            `json:"completed_last_7d"`
}

type activityStats struct {
	TotalEvents int             `json:"total_events"`
	Last7d      int             `json:"last_7d"`
	ByActor     []actorActivity `json:"by_actor"`
}

type actorActivity struct {
	Name         string `json:"name"`
	EventsLast7d int    `json:"events_last_7d"`
}

func (s *Server) systemStatsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Actors
	actors, err := s.svc.ListActors(ctx)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	as := actorStats{ByRole: map[string]int{}}
	for _, a := range actors {
		as.Total++
		if a.Active {
			as.Active++
		}
		as.ByRole[string(a.Role)]++
	}

	// Boards
	allBoards, err := s.svc.ListBoards(ctx, model.ListBoardsParams{IncludeDeleted: true})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	bs := boardStats{Total: len(allBoards)}
	for _, b := range allBoards {
		if !b.Deleted {
			bs.Active++
		}
	}

	// Tasks — aggregate across all active boards
	ts := taskStats{ByState: map[string]int{}}
	sevenDaysAgo := time.Now().UTC().AddDate(0, 0, -7)

	for _, b := range allBoards {
		if b.Deleted {
			continue
		}
		tasks, err := s.svc.ListTasks(ctx, model.TaskFilter{
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

		// Count completions from audit log (transitions to terminal states).
		audit, err := s.svc.QueryAuditByBoard(ctx, b.Slug)
		if err != nil {
			continue
		}
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
		}
	}

	// Activity — count audit events per actor in last 7 days
	actorEvents := map[string]int{}
	totalEvents := 0
	last7dEvents := 0

	for _, b := range allBoards {
		if b.Deleted {
			continue
		}
		audit, err := s.svc.QueryAuditByBoard(ctx, b.Slug)
		if err != nil {
			continue
		}
		totalEvents += len(audit)
		for _, e := range audit {
			if e.CreatedAt.After(sevenDaysAgo) {
				last7dEvents++
				actorEvents[e.Actor]++
			}
		}
	}

	var byActor []actorActivity
	for name, count := range actorEvents {
		byActor = append(byActor, actorActivity{Name: name, EventsLast7d: count})
	}

	stats := systemStats{
		Actors: as,
		Boards: bs,
		Tasks:  ts,
		Activity: activityStats{
			TotalEvents: totalEvents,
			Last7d:      last7dEvents,
			ByActor:     orEmpty(byActor),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		log.Printf("error encoding system stats: %v", err)
	}
}
