package team

// Member represents a single member of an agent team.
type Member struct {
	Name      string `json:"name"`
	AgentID   string `json:"agent_id"`
	AgentType string `json:"agent_type"` // "lead" or "teammate"
}

// TeamConfig is the on-disk structure of a team's config.json.
type TeamConfig struct {
	TeamName string   `json:"team_name"`
	Members  []Member `json:"members"`
}

// TaskState represents the status of a task.
type TaskState string

const (
	TaskPending    TaskState = "pending"
	TaskInProgress TaskState = "in_progress"
	TaskCompleted  TaskState = "completed"
)

// Task represents a single task in a team's task list.
type Task struct {
	ID        string    `json:"id"`
	Subject   string    `json:"subject"`
	Status    TaskState `json:"status"`
	Owner     string    `json:"owner,omitempty"`
	BlockedBy []string  `json:"blockedBy,omitempty"`
}

// TeamInfo is the aggregated view cached on each Agent.
type TeamInfo struct {
	TeamName        string
	MemberCount     int
	TotalTasks      int
	CompletedTasks  int
	InProgressTasks int
	PendingTasks    int
	Members         []Member
	Tasks           []Task
}
