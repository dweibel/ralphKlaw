package state

import (
	"fmt"
	"strings"
)

// Task represents a single item in TODO.md.
type Task struct {
	Description string
	Completed   bool
	Line        int
}

// parseTasks extracts tasks from TODO.md content.
func parseTasks(content string) []Task {
	var tasks []Task
	for i, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- [x] ") {
			tasks = append(tasks, Task{
				Description: strings.TrimPrefix(line, "- [x] "),
				Completed:   true,
				Line:        i,
			})
		} else if strings.HasPrefix(line, "- [ ] ") {
			tasks = append(tasks, Task{
				Description: strings.TrimPrefix(line, "- [ ] "),
				Completed:   false,
				Line:        i,
			})
		}
	}
	return tasks
}

// formatTasks renders tasks back to TODO.md markdown.
func formatTasks(tasks []Task) string {
	var buf strings.Builder
	buf.WriteString("# TODO\n\n")
	for _, t := range tasks {
		if t.Completed {
			buf.WriteString(fmt.Sprintf("- [x] %s\n", t.Description))
		} else {
			buf.WriteString(fmt.Sprintf("- [ ] %s\n", t.Description))
		}
	}
	return buf.String()
}
