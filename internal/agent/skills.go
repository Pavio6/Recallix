package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"gorm.io/gorm"

	"recallix/internal/repository"
	"recallix/internal/shared"
)

type githubSkillLocation struct {
	Owner string
	Repo  string
	Ref   string
	Path  string
}

type githubContentItem struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
}

type githubSkillFile struct {
	RelativePath string
	Content      []byte
}

func (s *Service) ListSkills(userID string) ([]repository.Skill, error) {
	var items []repository.Skill
	err := s.db.Where("user_id = ?", userID).
		Order("created_at desc").
		Find(&items).Error
	if err == nil {
		for i := range items {
			if err := s.db.Model(&repository.AgentSkill{}).
				Joins("JOIN agents ON agents.id = agent_skills.agent_id").
				Where("agent_skills.skill_id = ? AND agents.user_id = ?", items[i].ID, userID).
				Count(&items[i].AgentCount).Error; err != nil {
				return nil, err
			}
		}
	}
	if items == nil {
		items = []repository.Skill{}
	}
	return items, err
}

func (s *Service) ImportSkill(ctx context.Context, userID, githubURL string) (repository.Skill, error) {
	location, err := parseGitHubSkillURL(githubURL)
	if err != nil {
		return repository.Skill{}, err
	}

	files, err := fetchGitHubDirectory(ctx, location)
	if err != nil {
		return repository.Skill{}, err
	}
	if len(files) == 0 {
		return repository.Skill{}, fmt.Errorf("github skill directory is empty")
	}

	var entry *githubSkillFile
	for i := range files {
		if files[i].RelativePath == "SKILL.md" {
			entry = &files[i]
			break
		}
	}
	if entry == nil {
		return repository.Skill{}, fmt.Errorf("SKILL.md must exist at the skill directory root")
	}

	name, description, err := parseSkillMarkdown(string(entry.Content))
	if err != nil {
		return repository.Skill{}, err
	}

	now := time.Now()
	skillID := shared.NewID()
	storagePrefix := path.Join("skills", userID, skillID)
	var entryURI string
	for _, file := range files {
		objectKey := path.Join(storagePrefix, file.RelativePath)
		uri, err := s.store.Save(ctx, objectKey, "application/octet-stream", int64(len(file.Content)), bytes.NewReader(file.Content))
		if err != nil {
			return repository.Skill{}, err
		}
		if file.RelativePath == "SKILL.md" {
			entryURI = uri
		}
	}

	item := repository.Skill{
		ID:               skillID,
		UserID:           userID,
		Name:             name,
		Description:      description,
		SourceURL:        githubURL,
		SourceRepo:       location.Owner + "/" + location.Repo,
		SourceRef:        location.Ref,
		SourcePath:       location.Path,
		StoragePrefix:    storagePrefix,
		EntryFileURI:     entryURI,
		FileCount:        len(files),
		Enabled:          true,
		CreatedAt:        now,
		UpdatedAt:        now,
		LegacySourceType: "github",
	}
	if err := s.db.Create(&item).Error; err != nil {
		return repository.Skill{}, err
	}
	return item, nil
}

func (s *Service) DeleteSkill(ctx context.Context, userID, skillID string) error {
	var item repository.Skill
	if err := s.db.Where("id = ? AND user_id = ?", skillID, userID).First(&item).Error; err != nil {
		return err
	}
	if err := s.store.DeletePrefix(ctx, item.StoragePrefix); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("skill_id = ?", item.ID).Delete(&repository.AgentSkill{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&repository.Skill{}, "id = ? AND user_id = ?", item.ID, userID).Error; err != nil {
			return err
		}
		return nil
	})
}

func parseGitHubSkillURL(raw string) (githubSkillLocation, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return githubSkillLocation{}, err
	}
	if u.Scheme != "https" || u.Host != "github.com" {
		return githubSkillLocation{}, fmt.Errorf("only https://github.com skill directory URLs are supported")
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) < 5 || (segments[2] != "tree" && segments[2] != "blob") {
		return githubSkillLocation{}, fmt.Errorf("github URL must point to a repository skill directory")
	}
	sourcePath := strings.Join(segments[4:], "/")
	if segments[2] == "blob" && path.Base(sourcePath) == "SKILL.md" {
		sourcePath = path.Dir(sourcePath)
	}
	location := githubSkillLocation{
		Owner: segments[0],
		Repo:  segments[1],
		Ref:   segments[3],
		Path:  sourcePath,
	}
	if location.Owner == "" || location.Repo == "" || location.Ref == "" || location.Path == "" {
		return githubSkillLocation{}, fmt.Errorf("invalid github skill directory URL")
	}
	return location, nil
}

func fetchGitHubDirectory(ctx context.Context, location githubSkillLocation) ([]githubSkillFile, error) {
	return fetchGitHubDirectoryRecursive(ctx, location, location.Path)
}

func fetchGitHubDirectoryRecursive(ctx context.Context, location githubSkillLocation, currentPath string) ([]githubSkillFile, error) {
	apiURL := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		url.PathEscape(location.Owner),
		url.PathEscape(location.Repo),
		strings.TrimPrefix(path.Clean(currentPath), "/"),
		url.QueryEscape(location.Ref),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list github directory failed: %s", resp.Status)
	}
	var items []githubContentItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	var files []githubSkillFile
	for _, item := range items {
		switch item.Type {
		case "dir":
			children, err := fetchGitHubDirectoryRecursive(ctx, location, item.Path)
			if err != nil {
				return nil, err
			}
			files = append(files, children...)
		case "file":
			if item.DownloadURL == "" {
				return nil, fmt.Errorf("github file %q has no download url", item.Path)
			}
			content, err := downloadGitHubFile(ctx, item.DownloadURL)
			if err != nil {
				return nil, err
			}
			relativePath := strings.TrimPrefix(item.Path, strings.TrimSuffix(location.Path, "/")+"/")
			files = append(files, githubSkillFile{RelativePath: relativePath, Content: content})
		}
	}
	return files, nil
}

func downloadGitHubFile(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download github file failed: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

func parseSkillMarkdown(content string) (string, string, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return "", "", fmt.Errorf("skill frontmatter is required")
	}
	var name, description string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			break
		}
		switch {
		case strings.HasPrefix(line, "name:"):
			name = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "name:")), `"`)
		case strings.HasPrefix(line, "description:"):
			description = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "description:")), `"`)
		}
	}
	if strings.TrimSpace(name) == "" {
		return "", "", fmt.Errorf("skill name is required")
	}
	return name, description, nil
}
