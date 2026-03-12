package sandboxmatrix

// Sandbox represents a sandbox instance.
type Sandbox struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	Blueprint string `json:"blueprint"`
	RuntimeID string `json:"runtimeId"`
	IP        string `json:"ip,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// ExecResult holds the output of a command execution.
type ExecResult struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// Stats holds resource usage statistics for a sandbox.
type Stats struct {
	CPUUsage    float64 `json:"cpuUsage"`
	MemoryUsage uint64  `json:"memoryUsage"`
	MemoryLimit uint64  `json:"memoryLimit"`
}

// SnapshotResult holds the result of a snapshot creation.
type SnapshotResult struct {
	SnapshotID string `json:"snapshotId"`
	Tag        string `json:"tag"`
}

// Snapshot holds metadata about a point-in-time snapshot.
type Snapshot struct {
	ID        string `json:"id"`
	Tag       string `json:"tag"`
	CreatedAt string `json:"createdAt,omitempty"`
	Size      int64  `json:"size,omitempty"`
}

// Matrix represents a coordinated group of sandboxes.
type Matrix struct {
	Name    string         `json:"name"`
	State   string         `json:"state"`
	Members []MatrixMember `json:"members"`
}

// MatrixMember represents a member of a matrix.
type MatrixMember struct {
	Name      string `json:"name"`
	Blueprint string `json:"blueprint"`
}

// Session represents an interactive session bound to a sandbox.
type Session struct {
	ID        string `json:"id"`
	Sandbox   string `json:"sandbox"`
	State     string `json:"state"`
	ExecCount int    `json:"execCount,omitempty"`
}

// FileInfo represents metadata about a file in a sandbox.
type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	ModTime string `json:"modTime,omitempty"`
}

// VersionInfo holds server version information.
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
	OS        string `json:"os,omitempty"`
	Arch      string `json:"arch,omitempty"`
}

// HealthResponse holds the response from the health endpoint.
type HealthResponse struct {
	Status string `json:"status"`
}

// apiSandbox is the internal API representation returned by the server.
type apiSandbox struct {
	Metadata struct {
		Name      string `json:"name"`
		CreatedAt string `json:"createdAt"`
	} `json:"metadata"`
	Spec struct {
		BlueprintRef string `json:"blueprintRef"`
	} `json:"spec"`
	Status struct {
		State     string `json:"state"`
		RuntimeID string `json:"runtimeID"`
		IP        string `json:"ip"`
	} `json:"status"`
}

// parseSandbox converts an API sandbox response into a Sandbox.
func parseSandbox(a apiSandbox) Sandbox {
	return Sandbox{
		Name:      a.Metadata.Name,
		State:     a.Status.State,
		Blueprint: a.Spec.BlueprintRef,
		RuntimeID: a.Status.RuntimeID,
		IP:        a.Status.IP,
		CreatedAt: a.Metadata.CreatedAt,
	}
}
