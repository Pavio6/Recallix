package skill

import (
	"bufio"
	"strings"
)

func ExtractInstructions(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return strings.TrimSpace(content)
	}

	inFrontmatter := true
	var body []string
	for scanner.Scan() {
		line := scanner.Text()
		if inFrontmatter {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = false
			}
			continue
		}
		body = append(body, line)
	}
	if inFrontmatter {
		return strings.TrimSpace(content)
	}
	return strings.TrimSpace(strings.Join(body, "\n"))
}
