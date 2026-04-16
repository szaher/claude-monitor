package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
)

func (s *Server) handleFileHeatmap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	project := r.URL.Query().Get("project")
	if project == "" {
		writeError(w, http.StatusBadRequest, "missing required parameter 'project'")
		return
	}

	rows, err := s.db.Query(`
		SELECT tc.tool_name, tc.tool_input
		FROM tool_calls tc
		JOIN sessions s ON tc.session_id = s.id
		WHERE s.project_path = ?
		AND tc.tool_name IN ('Read', 'Write', 'Edit', 'Glob', 'Grep')
		AND tc.tool_input IS NOT NULL AND tc.tool_input != ''
	`, project)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query file heatmap: "+err.Error())
		return
	}
	defer rows.Close()

	type fileStats struct {
		Reads  int `json:"reads"`
		Writes int `json:"writes"`
		Edits  int `json:"edits"`
		Total  int `json:"total"`
	}

	files := map[string]*fileStats{}

	for rows.Next() {
		var toolName, toolInput string
		rows.Scan(&toolName, &toolInput)

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(toolInput), &parsed); err != nil {
			continue
		}

		filePath, _ := parsed["file_path"].(string)
		if filePath == "" {
			filePath, _ = parsed["path"].(string)
		}
		if filePath == "" {
			continue
		}

		if strings.HasPrefix(filePath, project) {
			filePath = strings.TrimPrefix(filePath, project)
			filePath = strings.TrimPrefix(filePath, "/")
		}

		if files[filePath] == nil {
			files[filePath] = &fileStats{}
		}
		switch toolName {
		case "Read":
			files[filePath].Reads++
		case "Write":
			files[filePath].Writes++
		case "Edit":
			files[filePath].Edits++
		}
		files[filePath].Total++
	}

	type fileEntry struct {
		Path string `json:"path"`
		fileStats
	}
	fileList := make([]fileEntry, 0, len(files))
	for path, stats := range files {
		fileList = append(fileList, fileEntry{Path: path, fileStats: *stats})
	}
	sort.Slice(fileList, func(i, j int) bool {
		return fileList[i].Total > fileList[j].Total
	})
	if len(fileList) > 50 {
		fileList = fileList[:50]
	}

	dirs := map[string]int{}
	for _, f := range fileList {
		dir := filepath.Dir(f.Path)
		dirs[dir] += f.Total
	}

	type dirEntry struct {
		Path  string `json:"path"`
		Total int    `json:"total"`
	}
	dirList := make([]dirEntry, 0, len(dirs))
	for path, total := range dirs {
		dirList = append(dirList, dirEntry{Path: path, Total: total})
	}
	sort.Slice(dirList, func(i, j int) bool {
		return dirList[i].Total > dirList[j].Total
	})
	if len(dirList) > 20 {
		dirList = dirList[:20]
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"files":       fileList,
		"directories": dirList,
	})
}
