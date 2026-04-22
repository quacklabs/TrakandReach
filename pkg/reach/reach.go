package reach

import (
	"github.com/username/trakand-reach/pkg/db"
	"github.com/username/trakand-reach/pkg/engine"
	"github.com/username/trakand-reach/pkg/models"
)

type Reach struct {
	Manager *engine.Manager
	Repo    *db.Repository
}

func New(dbPath string) (*Reach, error) {
	repo, err := db.NewRepository(dbPath)
	if err != nil {
		return nil, err
	}

	manager, err := engine.NewManager(repo)
	if err != nil {
		return nil, err
	}

	return &Reach{
		Manager: manager,
		Repo:    repo,
	}, nil
}

func (r *Reach) Start() error {
	return r.Manager.Start()
}

func (r *Reach) Stop() {
	r.Manager.Stop()
}

func (r *Reach) CreateSession(s *models.Session) (*engine.SessionInstance, error) {
	return r.Manager.StartSession(s)
}

func (r *Reach) SendMessage(sessionID, to, text string) error {
	return r.Manager.SendMessage(sessionID, to, text)
}
