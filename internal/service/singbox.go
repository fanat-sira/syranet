package service

import (
	"context"

	"errors"
	"github.com/fanat-sira/syranet/internal/config"
	"github.com/fanat-sira/syranet/internal/database"
	"github.com/fanat-sira/syranet/internal/models"
	"github.com/fanat-sira/syranet/internal/node"
	"sync"
	"time"
)

type Reconciler struct {
	mu      sync.Mutex
	Config  *config.Config
	Store   *database.Store
	Clients map[string]*node.Client
}

func NewReconciler(c *config.Config, s *database.Store) (*Reconciler, error) {
	r := &Reconciler{Config: c, Store: s, Clients: map[string]*node.Client{}}
	for _, n := range c.Nodes {
		client, e := node.NewClient(n.Agent.URL, n.Agent.Token, n.Agent.CAPath)
		if e != nil {
			return nil, e
		}
		r.Clients[n.ID] = client
	}
	return r, nil
}
func (r *Reconciler) Sync(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Store.LockedSnapshot(ctx, func(users []models.User, revision int64, record func(database.SyncState) error) error {
		var problems []error
		// Apply upstreams before dependents. Each failed node remains pending and retries.
		ordered := []config.Node{}
		seen := map[string]bool{}
		var visit func(config.Node)
		visit = func(n config.Node) {
			if seen[n.ID] {
				return
			}
			seen[n.ID] = true
			if n.Enabled && n.Outbound != "direct" {
				up, _ := r.Config.Node(n.Outbound)
				visit(up)
			}
			ordered = append(ordered, n)
		}
		for _, n := range r.Config.Nodes {
			visit(n)
		}
		for _, n := range ordered {
			state := database.SyncState{NodeID: n.ID, Revision: revision}
			spec, e := Generate(r.Config, n, users, time.Now().UTC())
			if e == nil {
				b, _ := node.Encode(spec)
				state.DesiredHash = node.Hash(b)
				status, err := r.Clients[n.ID].Status(ctx)
				if err != nil || !status.Running || !status.TLSReady || status.Hash != state.DesiredHash || status.Error != "" {
					status, err = r.Clients[n.ID].Apply(ctx, spec)
				}
				if err != nil {
					e = err
				} else if !status.Running || !status.TLSReady || status.Hash != state.DesiredHash {
					e = errors.New("node did not confirm desired config")
				} else {
					state.Healthy = true
					state.AppliedHash = status.Hash
					state.LastSuccess = &status.LastSuccess
				}
			}
			if e != nil {
				state.Error = "node sync failed; inspect agent, TLS and configuration"
				problems = append(problems, errors.New("sync failed for node "+n.ID))
			}
			if e = record(state); e != nil {
				return e
			}
		}
		return errors.Join(problems...)
	})
}
