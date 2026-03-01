package state

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: ralphklaw, Property 2: TODO.md parse/format round-trip
// Validates: Requirements 10.1, 10.2
func TestProperty_TODOParseFormatRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a list of tasks with non-empty descriptions (after trim)
		numTasks := rapid.IntRange(0, 20).Draw(t, "num_tasks")
		tasks := make([]Task, numTasks)
		
		for i := 0; i < numTasks; i++ {
			// Generate non-whitespace-only descriptions
			desc := rapid.StringMatching(`^[a-zA-Z0-9][a-zA-Z0-9 _-]*[a-zA-Z0-9]$`).Draw(t, "description")
			tasks[i] = Task{
				Description: desc,
				Completed:   rapid.Bool().Draw(t, "completed"),
				Line:        i,
			}
		}

		// Format to markdown
		formatted := formatTasks(tasks)

		// Parse back
		parsed := parseTasks(formatted)

		// Compare
		if len(parsed) != len(tasks) {
			t.Fatalf("length mismatch: got %d, want %d\nformatted:\n%s", len(parsed), len(tasks), formatted)
		}

		for i := range tasks {
			if parsed[i].Description != tasks[i].Description {
				t.Fatalf("description[%d] mismatch: got %q, want %q", i, parsed[i].Description, tasks[i].Description)
			}
			if parsed[i].Completed != tasks[i].Completed {
				t.Fatalf("completed[%d] mismatch: got %v, want %v", i, parsed[i].Completed, tasks[i].Completed)
			}
		}
	})
}

func TestParseTasks_Empty(t *testing.T) {
	tasks := parseTasks("")
	if len(tasks) != 0 {
		t.Errorf("expected empty list, got %d tasks", len(tasks))
	}
}

func TestParseTasks_SingleTask(t *testing.T) {
	content := "# TODO\n\n- [ ] Task one\n"
	tasks := parseTasks(content)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Description != "Task one" {
		t.Errorf("description = %q, want %q", tasks[0].Description, "Task one")
	}
	if tasks[0].Completed {
		t.Error("task should not be completed")
	}
}

func TestParseTasks_MultipleTasks_MixedCompletion(t *testing.T) {
	content := `# TODO

- [ ] Task one
- [x] Task two
- [ ] Task three
`
	tasks := parseTasks(content)

	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	if tasks[0].Completed {
		t.Error("task 0 should not be completed")
	}
	if !tasks[1].Completed {
		t.Error("task 1 should be completed")
	}
	if tasks[2].Completed {
		t.Error("task 2 should not be completed")
	}
}

func TestParseTasks_IgnoresNonTaskLines(t *testing.T) {
	content := `# TODO

Some text here

- [ ] Task one

More text

- [x] Task two
`
	tasks := parseTasks(content)

	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestFormatTasks_Empty(t *testing.T) {
	formatted := formatTasks([]Task{})
	if formatted != "# TODO\n\n" {
		t.Errorf("formatted = %q, want %q", formatted, "# TODO\n\n")
	}
}

func TestFormatTasks_SingleTask(t *testing.T) {
	tasks := []Task{
		{Description: "Test task", Completed: false},
	}
	formatted := formatTasks(tasks)

	expected := "# TODO\n\n- [ ] Test task\n"
	if formatted != expected {
		t.Errorf("formatted = %q, want %q", formatted, expected)
	}
}

func TestFormatTasks_CompletedTask(t *testing.T) {
	tasks := []Task{
		{Description: "Done task", Completed: true},
	}
	formatted := formatTasks(tasks)

	expected := "# TODO\n\n- [x] Done task\n"
	if formatted != expected {
		t.Errorf("formatted = %q, want %q", formatted, expected)
	}
}
