package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/szaher/claude-monitor/internal/db"
	"github.com/szaher/claude-monitor/internal/models"
)

func (s *Server) handleBudgets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listBudgets(w, r)
	case http.MethodPost:
		s.createBudget(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleBudgetDetail(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/budgets/")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "missing budget id")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid budget id")
		return
	}

	switch r.Method {
	case http.MethodPut:
		s.updateBudgetHandler(w, r, id)
	case http.MethodDelete:
		if err := db.DeleteBudget(s.db, id); err != nil {
			writeError(w, http.StatusInternalServerError, "delete budget: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) listBudgets(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query("SELECT id, name, COALESCE(project_path,''), period, amount_usd, enabled FROM budgets ORDER BY id DESC")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query budgets: "+err.Error())
		return
	}
	defer rows.Close()

	budgets := []map[string]interface{}{}
	for rows.Next() {
		var id int
		var name, projectPath, period string
		var amountUSD float64
		var enabled bool
		rows.Scan(&id, &name, &projectPath, &period, &amountUSD, &enabled)
		budgets = append(budgets, map[string]interface{}{
			"id": id, "name": name, "project_path": projectPath,
			"period": period, "amount_usd": amountUSD, "enabled": enabled,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"budgets": budgets})
}

func (s *Server) createBudget(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var b models.Budget
	if err := json.Unmarshal(body, &b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	b.Enabled = true
	id, err := db.InsertBudget(s.db, &b)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create budget: "+err.Error())
		return
	}
	b.ID = int(id)
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) updateBudgetHandler(w http.ResponseWriter, r *http.Request, id int) {
	body, _ := io.ReadAll(r.Body)
	var b models.Budget
	if err := json.Unmarshal(body, &b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	b.ID = id
	if err := db.UpdateBudget(s.db, &b); err != nil {
		writeError(w, http.StatusInternalServerError, "update budget: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) budgetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rows, err := s.db.Query("SELECT id, name, COALESCE(project_path,''), period, amount_usd, enabled FROM budgets WHERE enabled = 1")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query budgets: "+err.Error())
		return
	}
	defer rows.Close()

	now := time.Now().UTC()
	budgets := []map[string]interface{}{}

	for rows.Next() {
		var id int
		var name, projectPath, period string
		var amountUSD float64
		var enabled bool
		rows.Scan(&id, &name, &projectPath, &period, &amountUSD, &enabled)

		var periodStart string
		switch period {
		case "daily":
			periodStart = now.Format("2006-01-02")
		case "weekly":
			weekday := int(now.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			start := now.AddDate(0, 0, -(weekday - 1))
			periodStart = start.Format("2006-01-02")
		case "monthly":
			periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		}

		var currentSpend float64
		query := "SELECT COALESCE(SUM(estimated_cost_usd), 0) FROM sessions WHERE date(started_at) >= ?"
		qArgs := []interface{}{periodStart}
		if projectPath != "" {
			query += " AND project_path = ?"
			qArgs = append(qArgs, projectPath)
		}
		s.db.QueryRow(query, qArgs...).Scan(&currentSpend)

		pct := 0.0
		if amountUSD > 0 {
			pct = currentSpend / amountUSD * 100
		}

		status := "ok"
		if pct >= 100 {
			status = "exceeded"
		} else if pct >= 80 {
			status = "warning"
		}

		budgets = append(budgets, map[string]interface{}{
			"id": id, "name": name, "project_path": projectPath,
			"period": period, "amount_usd": amountUSD,
			"current_spend": currentSpend, "percentage": pct,
			"status": status,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"budgets": budgets})
}
