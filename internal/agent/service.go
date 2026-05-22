package agent

import (
	"context"
	"io"
	"strings"
	"time"

	"gorm.io/gorm"

	"recallix/internal/config"
	"recallix/internal/repository"
	"recallix/internal/shared"
	skillscheduler "recallix/internal/skill"
	"recallix/internal/storage"
)

const defaultPrompt = `You are Recallix in intelligent reasoning mode.
Use the provided knowledge base context as your primary evidence.
When the task is complex, reason through the problem step by step internally, compare relevant evidence, and produce a clear final answer.
If the available context is insufficient, say so honestly instead of inventing facts.
Follow any enabled skill instructions when they are relevant to the user's task.
Use proper markdown formatting and keep the answer concise but complete.`

type Service struct {
	db    *gorm.DB
	cfg   *config.Config
	store *storage.FileStorage
}

func (s *Service) Store() *storage.FileStorage {
	return s.store
}

func NewService(db *gorm.DB, cfg *config.Config, store *storage.FileStorage) *Service {
	return &Service{db: db, cfg: cfg, store: store}
}

func (s *Service) List(userID string) ([]repository.Agent, error) {
	var agents []repository.Agent
	if err := s.db.Where("user_id = ?", userID).Order("created_at desc").Find(&agents).Error; err != nil {
		return nil, err
	}
	for i := range agents {
		loaded, err := s.LoadRelations(agents[i])
		if err != nil {
			return nil, err
		}
		agents[i] = loaded
	}
	if agents == nil {
		agents = []repository.Agent{}
	}
	return agents, nil
}

func (s *Service) Get(userID, agentID string) (repository.Agent, error) {
	var a repository.Agent
	if err := s.db.Where("id = ? AND user_id = ?", agentID, userID).First(&a).Error; err != nil {
		return repository.Agent{}, err
	}
	return s.LoadRelations(a)
}

func (s *Service) Create(userID string, req UpsertAgentRequest) (repository.Agent, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "智能推理"
	}
	a := repository.Agent{
		ID:        shared.NewID(),
		UserID:    userID,
		Name:      name,
		Nickname:  strings.TrimSpace(req.Nickname),
		Model:     normalizedModel(req.Model, s.cfg.AgentModel),
		Prompt:    normalizedPrompt(req.Prompt),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.db.Create(&a).Error; err != nil {
		return repository.Agent{}, err
	}
	if err := s.replaceBindings(a.ID, req.SkillIDs); err != nil {
		return repository.Agent{}, err
	}
	return s.LoadRelations(a)
}

func (s *Service) Update(userID, agentID string, req UpsertAgentRequest) (repository.Agent, error) {
	a, err := s.Get(userID, agentID)
	if err != nil {
		return repository.Agent{}, err
	}
	if strings.TrimSpace(req.Name) != "" {
		a.Name = strings.TrimSpace(req.Name)
	}
	a.Nickname = strings.TrimSpace(req.Nickname)
	a.Model = normalizedModel(req.Model, a.Model)
	a.Prompt = normalizedPrompt(req.Prompt)
	a.UpdatedAt = time.Now()
	if err := s.db.Save(&a).Error; err != nil {
		return repository.Agent{}, err
	}
	if err := s.replaceBindings(a.ID, req.SkillIDs); err != nil {
		return repository.Agent{}, err
	}
	return s.LoadRelations(a)
}

func (s *Service) LoadRelations(a repository.Agent) (repository.Agent, error) {
	var skillIDs []string
	if err := s.db.Model(&repository.AgentSkill{}).Where("agent_id = ?", a.ID).Pluck("skill_id", &skillIDs).Error; err != nil {
		return a, err
	}
	if len(skillIDs) > 0 {
		if err := s.db.Where("id IN ? AND user_id = ? AND enabled = ?", skillIDs, a.UserID, true).Find(&a.Skills).Error; err != nil {
			return a, err
		}
	}
	return a, nil
}

func (s *Service) replaceBindings(agentID string, skillIDs []string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("agent_id = ?", agentID).Delete(&repository.AgentSkill{}).Error; err != nil {
			return err
		}
		for _, id := range skillIDs {
			if err := tx.Create(&repository.AgentSkill{AgentID: agentID, SkillID: id}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

type UpsertAgentRequest struct {
	Name     string   `json:"name"`
	Nickname string   `json:"nickname"`
	Model    string   `json:"model"`
	Prompt   string   `json:"prompt"`
	SkillIDs []string `json:"skill_ids"`
}

func normalizedModel(model, fallback string) string {
	if strings.TrimSpace(model) == "" {
		return fallback
	}
	return strings.TrimSpace(model)
}

func normalizedPrompt(prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return defaultPrompt
	}
	return strings.TrimSpace(prompt)
}

func (s *Service) BuildLoadedSkillContext(ctx context.Context, selected []skillscheduler.Candidate) string {
	if len(selected) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("【Loaded Skill Instructions】\n")
	sb.WriteString("Follow the loaded skill instructions below when they are relevant to the user's request.\n")
	for _, candidate := range selected {
		reader, err := s.store.Open(ctx, candidate.Skill.EntryFileURI)
		if err != nil {
			continue
		}
		content, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			continue
		}
		instructions := skillscheduler.ExtractInstructions(string(content))
		if strings.TrimSpace(instructions) == "" {
			continue
		}
		sb.WriteString("\n## ")
		sb.WriteString(candidate.Skill.Name)
		if candidate.Skill.Description != "" {
			sb.WriteString("\n")
			sb.WriteString(candidate.Skill.Description)
		}
		sb.WriteString("\n")
		sb.WriteString(instructions)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}
